package river_service

import (
	"bytes"
	"io"
	"os"
	"path"
	"sync"
	"time"

	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/pingcap/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// masterInfo 存储MySQL主库位置信息
type masterInfo struct {
	sync.RWMutex

	Name string `yaml:"bin_name"` // binlog文件名
	Pos  uint32 `yaml:"bin_pos"`  // binlog位置

	filePath     string    // 文件路径
	lastSaveTime time.Time // 最后保存时间
	logger       *logrus.Logger
}

// loadMasterInfo 从指定目录加载master信息
func loadMasterInfo(dataDir string, logger *logrus.Logger) (*masterInfo, error) {
	var m masterInfo
	m.logger = logger

	if len(dataDir) == 0 {
		return &m, nil
	}

	m.filePath = path.Join(dataDir, "master.yaml")
	m.lastSaveTime = time.Now()

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, errors.Trace(err)
	}

	f, err := os.Open(m.filePath)
	if err != nil && !os.IsNotExist(errors.Cause(err)) {
		return nil, errors.Trace(err)
	} else if os.IsNotExist(errors.Cause(err)) {
		return &m, nil
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&m)
	return &m, errors.Trace(err)
}

// Save 保存MySQL位置信息到文件
func (m *masterInfo) Save(pos mysql.Position) error {
	if m.logger != nil {
		m.logger.Debugf("保存同步位点: %s", pos)
	}

	m.Lock()
	defer m.Unlock()

	m.Name = pos.Name
	m.Pos = pos.Pos

	var err error

	if len(m.filePath) == 0 {
		return nil
	}

	n := time.Now()
	if n.Sub(m.lastSaveTime) < time.Second {
		return nil
	}

	m.lastSaveTime = n
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)

	err = encoder.Encode(m)
	if err != nil {
		encoder.Close()
		return errors.Trace(err)
	}
	err = encoder.Close()
	if err != nil {
		return errors.Trace(err)
	}

	if err = WriteFileAtomic(m.filePath, buf.Bytes(), 0644); err != nil {
		if m.logger != nil {
			m.logger.Errorf("保存 Canal 主库位点文件失败: 文件=%s 错误=%v", m.filePath, err)
		}
	}

	return errors.Trace(err)
}

// Position 返回当前MySQL位置信息
func (m *masterInfo) Position() mysql.Position {
	m.RLock()
	defer m.RUnlock()

	return mysql.Position{
		Name: m.Name,
		Pos:  m.Pos,
	}
}

// Close 关闭master信息，保存当前位置
func (m *masterInfo) Close() error {
	pos := m.Position()

	return m.Save(pos)
}

func WriteFileAtomic(filename string, data []byte, perm os.FileMode) error {
	dir, name := path.Dir(filename), path.Base(filename)
	f, err := os.CreateTemp(dir, name)
	if err != nil {
		return err
	}
	n, err := f.Write(data)
	f.Close()
	if err == nil && n < len(data) {
		err = io.ErrShortWrite
	} else {
		err = os.Chmod(f.Name(), perm)
	}
	if err != nil {
		os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), filename)
}
