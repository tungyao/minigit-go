package main

// 状态常量定义
const (
	StatusNormal = 0 // 正常状态（已跟踪，未修改）
	StatusAdd    = 1 // 新增文件
	StatusModify = 2 // 修改文件
	StatusDelete = 3 // 删除文件
)

type Tree struct {
	Hash      string
	Path      string
	Name      string
	Timestamp string
	Status    int
}
type HeadObj struct {
	ParentHash string
	Message    string
	Timestamp  string
	Trees      []Tree
}

// 对于配置文件 config 的结构定义
// 用于push和pull操作
type Conf struct {
	Remote   map[string]string // name -> url
	Password map[string]string // name -> password
}
