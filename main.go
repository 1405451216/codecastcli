package main

import (
	"fmt"
	"os"

	"codecast/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		// 检查是否是 ExitCodeError，按其指定退出码退出
		if exitErr, ok := err.(*cmd.ExitCodeError); ok {
			if exitErr.Err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", exitErr.Err)
			}
			os.Exit(exitErr.Code)
		}
		// 其他错误统一返回 1
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
