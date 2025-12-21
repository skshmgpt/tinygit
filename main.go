package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	args := os.Args
	switch args[1] {
	case "init":
		info, err := os.Stat(".tinygit")
		if err == nil && info.IsDir() {
			cwd, _ := os.Getwd()
			msg := fmt.Sprintf("Reinitialized existing tinygit repository in %s/%s\n", cwd, ".tinygit")
			fmt.Print(msg)
		} else if os.IsNotExist(err) {
			_ = os.MkdirAll(".tinygit/objects", 0755)
			_ = os.MkdirAll(".tinygit/refs/heads", 0755)
			head, _ := os.Create(".tinygit/HEAD")
			_, err := head.WriteString("ref: refs/heads/main")
			if err != nil {
				log.Fatalf("Error writing to HEAD : %s", err.Error())
			}
		}
	}
}
