// internal/builder/gockerfile.go
package builder

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Instruction 代表 Gockerfile 中的一條指令
type Instruction struct {
	Command string   // FROM, RUN, CMD, ENTRYPOINT, ADD, COPY
	Args    []string // 指令參數
	RawLine string   // 原始行內容
}

// Gockerfile 代表解析後的 Gockerfile
type Gockerfile struct {
	Instructions []Instruction
	BaseImage    string   // FROM 指令指定的基礎映像
	RunCommands  []string // RUN 指令
	Cmd          []string // CMD 指令
	Entrypoint   []string // ENTRYPOINT 指令
	Copies       []CopyInstruction
	Adds         []AddInstruction
}

// CopyInstruction COPY 指令的詳細資訊
type CopyInstruction struct {
	Source string
	Dest   string
}

// AddInstruction ADD 指令的詳細資訊
type AddInstruction struct {
	Source string
	Dest   string
}

// ParseGockerfile 解析 Gockerfile
func ParseGockerfile(filepath string) (*Gockerfile, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("無法開啟 Gockerfile: %w", err)
	}
	defer file.Close()

	gf := &Gockerfile{
		Instructions: []Instruction{},
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// 跳過空行和註解
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析指令
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		cmd := strings.ToUpper(parts[0])
		args := parts[1:]

		instruction := Instruction{
			Command: cmd,
			Args:    args,
			RawLine: line,
		}
		gf.Instructions = append(gf.Instructions, instruction)

		// 根據指令類型處理
		switch cmd {
		case "FROM":
			if len(args) == 0 {
				return nil, fmt.Errorf("第 %d 行: FROM 指令需要指定映像名稱", lineNum)
			}
			gf.BaseImage = args[0]

		case "RUN":
			if len(args) == 0 {
				return nil, fmt.Errorf("第 %d 行: RUN 指令需要指定命令", lineNum)
			}
			gf.RunCommands = append(gf.RunCommands, strings.Join(args, " "))

		case "CMD":
			if len(args) == 0 {
				return nil, fmt.Errorf("第 %d 行: CMD 指令需要指定命令", lineNum)
			}
			// 檢查是否為 JSON 陣列格式
			if strings.HasPrefix(line[3:], " [") || strings.HasPrefix(line[3:], "[") {
				cmdLine := strings.TrimSpace(line[3:])
				var cmdArray []string
				if err := json.Unmarshal([]byte(cmdLine), &cmdArray); err != nil {
					return nil, fmt.Errorf("第 %d 行: CMD JSON 格式錯誤: %w", lineNum, err)
				}
				gf.Cmd = cmdArray
			} else {
				gf.Cmd = args
			}

		case "ENTRYPOINT":
			if len(args) == 0 {
				return nil, fmt.Errorf("第 %d 行: ENTRYPOINT 指令需要指定命令", lineNum)
			}
			// 檢查是否為 JSON 陣列格式
			if strings.HasPrefix(line[10:], " [") || strings.HasPrefix(line[10:], "[") {
				epLine := strings.TrimSpace(line[10:])
				var epArray []string
				if err := json.Unmarshal([]byte(epLine), &epArray); err != nil {
					return nil, fmt.Errorf("第 %d 行: ENTRYPOINT JSON 格式錯誤: %w", lineNum, err)
				}
				gf.Entrypoint = epArray
			} else {
				gf.Entrypoint = args
			}

		case "COPY":
			if len(args) < 2 {
				return nil, fmt.Errorf("第 %d 行: COPY 指令需要來源和目標路徑", lineNum)
			}
			gf.Copies = append(gf.Copies, CopyInstruction{
				Source: args[0],
				Dest:   args[1],
			})

		case "ADD":
			if len(args) < 2 {
				return nil, fmt.Errorf("第 %d 行: ADD 指令需要來源和目標路徑", lineNum)
			}
			gf.Adds = append(gf.Adds, AddInstruction{
				Source: args[0],
				Dest:   args[1],
			})

		default:
			return nil, fmt.Errorf("第 %d 行: 不支援的指令 '%s'", lineNum, cmd)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("讀取 Gockerfile 失敗: %w", err)
	}

	// 驗證至少有 FROM 指令
	if gf.BaseImage == "" {
		return nil, fmt.Errorf("Gockerfile 必須包含 FROM 指令")
	}

	return gf, nil
}
