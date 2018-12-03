// thermal-uploader - upload thermal video recordings in CPTV format to the project's API server.
//  Copyright (C) 2017, The Cacophony Project
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	arg "github.com/alexflint/go-arg"
	"github.com/rjeczalik/notify"
)

const cptvGlob = "*.cptv"
const failedUploadsDir = "failed-uploads"

var version = "No version provided"

type Args struct {
	ConfigFile string `arg:"-c,--config" help:"path to configuration file"`
}

func (Args) Version() string {
	return version
}

func procArgs() Args {
	var args Args
	args.ConfigFile = "/etc/thermal-uploader.yaml"
	arg.MustParse(&args)
	return args
}

func main() {
	err := runMain()
	if err != nil {
		log.Fatal(err.Error())
	}
}

func runMain() error {
	log.SetFlags(0) // Removes default timestamp flag

	args := procArgs()
	log.Printf("running version: %s", version)
	conf, err := ParseConfigFile(args.ConfigFile)
	if err != nil {
		return fmt.Errorf("configuration error: %v", err)
	}
	privConfigFilename := genPrivConfigFilename(args.ConfigFile)
	log.Println("private settings file:", privConfigFilename)
	password, err := ReadPassword(privConfigFilename)
	if err != nil {
		return err
	}
	api, err := NewAPI(conf.ServerURL, conf.Group, conf.DeviceName, conf.UserName, password)
	if err != nil {
		return err
	}
	if api.JustRegistered() {
		log.Println("first time registration - saving password")
		err := WritePassword(privConfigFilename, api.Password())
		if err != nil {
			return err
		}
	}

	log.Println("making failed uploads directory")
	os.MkdirAll(filepath.Join(conf.Directory, failedUploadsDir), 0755)

	log.Println("watching", conf.Directory)
	fsEvents := make(chan notify.EventInfo, 1)
	if err := notify.Watch(conf.Directory, fsEvents, notify.InCloseWrite, notify.InMovedTo); err != nil {
		return err
	}
	defer notify.Stop(fsEvents)
	for {
		// Check for files to upload first in case there are CPTV
		// files around when the uploader starts.
		if err := uploadFiles(api, conf.Directory); err != nil {
			return err
		}
		// Block until there's activity in the directory. We don't
		// care what it is as uploadFiles will only act on CPTV
		// files.
		<-fsEvents
	}
	return nil
}

func genPrivConfigFilename(confFilename string) string {
	dirname, filename := filepath.Split(confFilename)
	bareFilename := strings.TrimSuffix(filename, ".yaml")
	return filepath.Join(dirname, bareFilename+"-priv.yaml")
}

func uploadFiles(api *CacophonyAPI, directory string) error {
	matches, _ := filepath.Glob(filepath.Join(directory, cptvGlob))
	for _, filename := range matches {
		err := uploadFileWithRetries(api, filename)
		if err != nil {
			return err
		}
	}
	return nil
}

func uploadFileWithRetries(api *CacophonyAPI, filename string) error {
	log.Printf("uploading: %s", filename)

	for remainingTries := 2; remainingTries >= 0; remainingTries-- {
		err := uploadFile(api, filename)
		if err == nil {
			log.Printf("upload complete: %s", filename)
			os.Remove(filename)
			return nil
		}
		if remainingTries >= 1 {
			log.Printf("upload failed, trying %d more times", remainingTries)
		}
	}
	log.Printf("upload failed multiple times, moving file to failed uploads folder")
	dir, name := filepath.Split(filename)
	return os.Rename(filename, filepath.Join(dir, failedUploadsDir, name))
}

func uploadFile(api *CacophonyAPI, filename string) error {
	f, err := os.Open(filename)
	if os.IsNotExist(err) {
		// File disappeared since the event was generated. Ignore.
		return nil
	} else if err != nil {
		return err
	}
	defer f.Close()
	return api.UploadThermalRaw(bufio.NewReader(f))
}
