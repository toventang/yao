package widget

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/yaoapp/gou"
	"github.com/yaoapp/gou/widget"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/config"
	"github.com/yaoapp/yao/share"
	"github.com/yaoapp/yao/table"
)

// Load Widgets
func Load(cfg config.Config) error {
	var root = filepath.Join(cfg.Root, "widgets")
	return LoadFrom(root)
}

// LoadFrom widget
func LoadFrom(dir string) error {

	register := moduleRegister()

	if share.DirNotExists(dir) {
		return fmt.Errorf("%s does not exists", dir)
	}

	paths, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, path := range paths {

		if !path.IsDir() {
			continue
		}

		name := path.Name()
		if _, err := os.Stat(filepath.Join(dir, name, "widget.json")); errors.Is(err, os.ErrNotExist) {
			// path/to/whatever does not exist
			continue
		}
		w, err := gou.LoadWidget(filepath.Join(dir, name), name, register)
		if err != nil {
			return err
		}

		// Load instances
		err = w.Load()
		if err != nil {
			return err
		}
	}

	return err
}

func moduleRegister() widget.ModuleRegister {
	return widget.ModuleRegister{
		"Apis": func(name string, source []byte) error {
			_, err := gou.LoadAPIReturn(string(source), name)
			log.Trace("[Widget] Register api %s", name)
			if err != nil {
				log.Error("[Widget] Register api %s %v", name, err)
			}
			return err
		},
		"Models": func(name string, source []byte) error {
			_, err := gou.LoadModelReturn(string(source), name)
			log.Trace("[Widget] Register model %s", name)
			if err != nil {
				log.Error("[Widget] Register model %s %v", name, err)
			}
			return err
		},
		"Tables": func(name string, source []byte) error {
			log.Trace("[Widget] Register table %s", name)
			_, err := table.LoadTable(string(source), name)
			if err != nil {
				log.Error("[Widget] Register table %s %v", name, err)
			}
			return nil
		},
		"Tasks": func(name string, source []byte) error {
			log.Trace("[Widget] Register task %s", name)
			_, err := gou.LoadTask(string(source), name)
			if err != nil {
				log.Error("[Widget] Register task %s %v", name, err)
			}
			return nil
		},
		"Schedules": func(name string, source []byte) error {
			log.Trace("[Widget] Register schedule %s", name)
			_, err := gou.LoadSchedule(string(source), name)
			if err != nil {
				log.Error("[Widget] Register schedule %s %v", name, err)
			}
			return nil
		},
	}
}
