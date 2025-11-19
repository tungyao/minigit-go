package main

import (
	"encoding/json"
	"os"
)

const (
	Mark = ".minigit"
)

type LocalManager struct{}

func (l *LocalManager) GetRoot() (string, error) {
	root, err := os.Getwd()
	// log.Println("root:", root)
	return root, err
}
func (m *LocalManager) GetHead() (string, error) {
	root, err := m.GetRoot()
	if err != nil {
		return "", err
	}
	return root + "/" + Mark + "/head", nil
}
func (m *LocalManager) GetIndex() (string, error) {
	root, err := m.GetRoot()
	if err != nil {
		return "", err
	}
	return root + "/" + Mark + "/index", nil
}
func (m *LocalManager) GetIndexContent() (string, error) {
	index, err := m.GetIndex()
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(index)
	return string(content), err
}

func (m *LocalManager) GetHeadContent() (string, error) {
	headFile, err := m.GetHead()
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(headFile)
	return string(content), err
}
func (m *LocalManager) FlushHead(head string) error {
	headFile, err := m.GetHead()
	if err != nil {
		return err
	}
	return os.WriteFile(headFile, []byte(head), os.ModePerm)
}
func (m *LocalManager) FlushIndex(tree []Tree) error {
	indexFile, err := m.GetIndex()
	if err != nil {
		return err
	}
	if tree == nil {
		return os.WriteFile(indexFile, []byte(""), os.ModePerm)
	}
	// 处理json
	prettyJSON, err := json.MarshalIndent(tree, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexFile, prettyJSON, os.ModePerm)
}
func (m *LocalManager) GetObjectFileContent(hash string) ([]byte, error) {
	content, err := os.ReadFile(hash)
	return content, err
}

func (m *LocalManager) IsInit() bool {
	head, err := m.GetHead()
	if err != nil {
		return false
	}
	_, err = os.Stat(head)
	return err == nil
}

// SaveBlob 保存文件内容到objects目录
func (m *LocalManager) SaveBlob(hash string, content []byte) error {
	root, err := m.GetRoot()
	if err != nil {
		return err
	}

	// 使用hash的前2个字符作为子目录名
	objectDir := root + "/" + Mark + "/objects/" + hash[:2]
	err = os.MkdirAll(objectDir, 0755)
	if err != nil {
		return err
	}

	// 保存文件内容
	objectPath := objectDir + "/" + hash[2:]
	return os.WriteFile(objectPath, content, 0644)
}

// GetBlob 读取文件内容从objects目录
func (m *LocalManager) GetBlob(hash string) ([]byte, error) {
	root, err := m.GetRoot()
	if err != nil {
		return nil, err
	}

	objectPath := root + "/" + Mark + "/objects/" + hash[:2] + "/" + hash[2:]
	return os.ReadFile(objectPath)
}

// GetCommitsFile 获取commits文件路径
func (m *LocalManager) GetCommitsFile() (string, error) {
	root, err := m.GetRoot()
	if err != nil {
		return "", err
	}
	return root + "/" + Mark + "/commits", nil
}

// CommitRef 提交引用信息
type CommitRef struct {
	Hash      string `json:"hash"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// GetAllCommits 获取所有提交引用
func (m *LocalManager) GetAllCommits() ([]CommitRef, error) {
	commitsFile, err := m.GetCommitsFile()
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(commitsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []CommitRef{}, nil
		}
		return nil, err
	}

	if len(content) == 0 {
		return []CommitRef{}, nil
	}

	var commits []CommitRef
	err = json.Unmarshal(content, &commits)
	if err != nil {
		return nil, err
	}

	return commits, nil
}

// AddCommit 添加新提交到commits文件
func (m *LocalManager) AddCommit(hash, message, timestamp string) error {
	commits, err := m.GetAllCommits()
	if err != nil {
		return err
	}

	// 检查是否已存在
	for _, commit := range commits {
		if commit.Hash == hash {
			return nil // 已存在，不重复添加
		}
	}

	// 添加新提交（插入到开头，最新的在前）
	newCommit := CommitRef{
		Hash:      hash,
		Message:   message,
		Timestamp: timestamp,
	}
	commits = append([]CommitRef{newCommit}, commits...)

	// 保存到文件
	commitsFile, err := m.GetCommitsFile()
	if err != nil {
		return err
	}

	prettyJSON, err := json.MarshalIndent(commits, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(commitsFile, prettyJSON, 0644)
}
