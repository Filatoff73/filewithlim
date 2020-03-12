package fileWithLim

import (
	"MediaCore/internal/pkg/utils"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileLim struct {
	filepath        string
	limit           int64 //in bytes
	perm            os.FileMode
	flag            int
	currFile        *os.File
	mutex           *sync.Mutex
	zipMutex        *sync.Mutex
	maxLogFileCount int
}

func OpenFile(filepath_ string, flag_ int, perm_ os.FileMode, limit_ int64) (*FileLim, error) {

	fpath := filepath_
	fpathAbs, err := filepath.Abs(filepath_)
	if err == nil {
		fpath = fpathAbs
	}

	_ = os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
	f, err := os.OpenFile(fpath, flag_, perm_)
	if err != nil {
		return nil, err
	}
	res := &FileLim{
		filepath:        fpath,
		limit:           limit_,
		perm:            perm_,
		flag:            flag_,
		currFile:        f,
		mutex:           &sync.Mutex{},
		zipMutex:        &sync.Mutex{},
		maxLogFileCount: 50,
	}

	return res, nil
}

func (f *FileLim) Write(p []byte) (int, error) {

	f.mutex.Lock()
	defer f.mutex.Unlock()
	n, err := f.currFile.Write(p)
	if err != nil {
		return n, err
	}
	_ = f.checkSize()
	return n, err
}

func (f *FileLim) checkSize() error {

	fi, err := f.currFile.Stat()
	if err != nil {
		return err
	}
	fileSize := fi.Size()
	if fileSize > f.limit {
		//переименовываем текущий файл
		f.currFile.Close()
		curTime := time.Now()
		timeStr := fmt.Sprintf("_%d_%02d_%02dT%02d_%02d_%02d",
			curTime.Year(), curTime.Month(), curTime.Day(),
			curTime.Hour(), curTime.Minute(), curTime.Second())
		newName := f.filepath + timeStr
		err := os.Rename(f.filepath, newName)
		if err != nil {
			f.currFile, _ = os.OpenFile(f.filepath, f.flag, f.perm)
			return err
		}
		//открваем новый файл, куда будут писаться логи
		emptyFile, err := os.Create(f.filepath)
		if err != nil {
			return err
		}
		emptyFile.Close()
		f.currFile, err = os.OpenFile(f.filepath, f.flag, f.perm)
		if err != nil {
			return err
		}
		//сжимаем старый файл
		go f.zipFileAndCheckLogCount(newName)
	}
	return nil
}

func (f *FileLim) Close() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.currFile.Close()
}

func (f *FileLim) zipFileAndCheckLogCount(fPath string) error {
	f.zipMutex.Lock()
	defer f.zipMutex.Unlock()
	_ = f.zipFile(fPath)
	_ = f.checkLogsCount()
	return nil
}

func (f *FileLim) zipFile(fPath string) error {
	content, err := ioutil.ReadFile(fPath)
	if err != nil {
		return err
	}
	zippedContent, err := utils.Zip(content)
	if err != nil {
		return err
	}
	newNameZipped := fPath + ".zip"
	err = ioutil.WriteFile(newNameZipped, zippedContent, os.ModePerm)
	if err != nil {
		return err
	}
	err = os.Remove(fPath)
	return err
}

func (f *FileLim) checkLogsCount() error {
	if f.maxLogFileCount < 2 {
		return nil
	}
	dir := filepath.Dir(f.filepath)

	files, err := utils.ReadDir(dir, -1)
	if err != nil {
		return err
	}
	lenFiles := len(files)
	needDeleteFiles := lenFiles - f.maxLogFileCount

	for i := 0; i < lenFiles && needDeleteFiles > 0; i++ {
		if files[i].Name() != filepath.Base(f.filepath) {
			err = os.Remove(filepath.Join(dir, files[i].Name()))
			if err != nil {
				return err
			}
			needDeleteFiles--
		}
	}

	return err
}
