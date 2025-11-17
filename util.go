package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// 获取head

// 获取文件hash
func GetFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha1.New()

	// 64KB buffer provides a good balance between memory usage and speed
	buffer := make([]byte, 64*1024)

	for {
		bytesRead, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return "", err
		}

		if bytesRead == 0 {
			break
		}

		_, writeErr := hasher.Write(buffer[:bytesRead])
		if writeErr != nil {
			return "", writeErr
		}
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// 获取文件绝对路径
func GetFileAbsPath(filePath string) (string, error) {
	if filePath == "" {
		return "", fmt.Errorf("file path cannot be empty")
	}

	// Handle the special case of "." to ensure it resolves to current directory
	if filePath == "." {
		currentDir, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		return currentDir, nil
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	return absPath, nil
}

// GetRelativePath calculates the relative path of a given target path
// (which can be a file or directory) with respect to the current working directory.
// The targetPath can be either absolute or relative.
// It returns the relative path as a string and any error encountered.
func GetRelativePath(targetPath string) (string, error) {
	// 1. Get the absolute path of the current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	// 2. Ensure the targetPath is absolute
	// If it's already absolute, Abs() returns it unchanged (on most systems).
	// If it's relative, Abs() resolves it based on the current working directory.
	targetAbsPath, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for target '%s': %w", targetPath, err)
	}

	// 3. Calculate the relative path from currentDir to targetAbsPath
	relPath, err := filepath.Rel(currentDir, targetAbsPath)
	if err != nil {
		return "", fmt.Errorf("failed to calculate relative path from '%s' to '%s': %w", currentDir, targetAbsPath, err)
	}

	return relPath, nil
}

// PathType 判断给定路径是文件还是目录
// 返回值:
//   - "file": 路径是一个普通文件
//   - "dir": 路径是一个目录
//   - "other": 路径存在但不是普通文件也不是目录 (例如设备、管道等)
//   - "": 路径不存在或发生错误
func PathType(path string) string {
	// 使用 os.Stat 获取文件/目录信息
	// 如果需要区分符号链接本身和它指向的内容，可以使用 os.Lstat
	info, err := os.Stat(path)
	if err != nil {
		// 检查错误类型
		if os.IsNotExist(err) {
			// 路径不存在
			fmt.Printf("Path %s does not exist.\n", path)
		} else {
			// 其他错误，例如权限问题
			fmt.Printf("Error accessing path %s: %v\n", path, err)
		}
		return "" // 或者你可以选择 panic(err) 或返回错误
	}

	// 检查是否为目录
	if info.IsDir() {
		return "dir"
	} else {
		// 如果不是目录，则通常认为是文件（尽管也可能是其他类型）
		// 可以进一步检查 Mode() 来区分普通文件和其他（如设备、FIFO等）
		// 这里简化处理，非目录即视为文件
		mode := info.Mode()
		if mode.IsRegular() { // 更精确地判断是否为普通文件
			return "file"
		}
		// 如果不是普通文件也不是目录，可以根据需要处理
		// 例如：mode&os.ModeDevice != 0 表示设备文件
		//      mode&os.ModeNamedPipe != 0 表示命名管道
		//      mode&os.ModeSymlink != 0 表示符号链接 (如果用Lstat)
		//      mode&os.ModeSocket != 0 表示套接字
		return "other" // 或者也可以返回 "file"，取决于你的具体需求

	}
}
