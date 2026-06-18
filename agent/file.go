package agent

import (
	"fmt"
	"os"
	"strings"

	"k8s.io/klog/v2"
)

func (nm *NodeConfigManager) CheckFiles() (diffMsg string, checkItems string, err error) {

	// 先遍历np中配置的file
	for index, fm := range nm.NodeGroup.Spec.FileManagers {
		containerFilePath := fm.Path
		if nm.ContainerHostRootPrefix != "" {
			// 说明在容器中
			containerFilePath = fmt.Sprintf("%v/%v", nm.ContainerHostRootPrefix, containerFilePath)
		}
		checkItems = fmt.Sprintf("%v,[index=%v path=%v]", checkItems, index, fm.Path)
		var (
			actualBs []byte
		)
		actualBs, err = os.ReadFile(containerFilePath)
		if err != nil {
			klog.ErrorS(err, "CheckFiles.ReadFile.err",
				"containerFilePath", containerFilePath,
			)
			return
		}

		// 按行读取 对比内容
		desiredString := fm.Content
		actualStringLines := strings.Split(strings.TrimSpace(string(actualBs)), "\n")
		desiredStringLines := strings.Split(desiredString, "\n")
		// 开头就要判断2边的行数是否一致
		if len(actualStringLines) != len(desiredStringLines) {
			diffMsg = fmt.Sprintf("%v\n[file.line.diff][file=%v  desired=%v actual=%v]",
				diffMsg,
				fm.Path,
				len(desiredStringLines),
				len(actualStringLines),
			)
			return
		}
		// 一致的时候再按行读取
		// 开始按行读取日志
		desiredLines := len(desiredStringLines)
		for i := 0; i < desiredLines; i++ {
			desiredLine := desiredStringLines[i]
			actualLine := actualStringLines[i]
			if desiredLine != actualLine {
				diffMsg = fmt.Sprintf("%v\n[file.line.missmatch][file=%v line=%v desired=%v actual=%v]",
					diffMsg,
					fm.Path,
					i,
					desiredLine,
					actualLine,
				)
			}
		}

	}
	return
}
