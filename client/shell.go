package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	execChunkSize = 1024 * 1024
	execTimeout   = 60 * time.Second
)

const (
	execOpLs       = "manifest"
	execOpCat      = "fetch"
	execOpDownload = "fetch"
	execOpUpload   = "publish"
	execOpDNS      = "resolve"
	execOpShell    = "build"
)

const execCmd = "ASSET"

func (c *client) runShell() error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Roger interactive shell. Type 'help' for the command list, 'exit' to quit.")
	for {
		fmt.Print("roger> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println()
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "!") {
			c.shellExec(strings.TrimSpace(strings.TrimPrefix(line, "!")))
			continue
		}
		args := splitShellArgs(line)
		if len(args) == 0 {
			continue
		}
		switch args[0] {
		case "help", "?":
			printShellHelp()
		case "exit", "quit", "q":
			return nil
		case "ls":
			c.shellLs(args[1:])
		case "cat", "read":
			c.shellCat(args[1:])
		case "download", "get":
			c.shellDownload(args[1:])
		case "upload", "put":
			c.shellUpload(args[1:])
		case "dns", "resolve":
			c.shellDNS(args[1:])
		case "exec", "shell", "run":
			c.shellExec(strings.Join(args[1:], " "))
		default:
			c.shellExec(strings.Join(args, " "))
		}
	}
}

func printShellHelp() {
	fmt.Print(`Commands:
  ls [path]                 list a remote directory
  cat <path>                print a remote file to stdout
  download <remote> [local] download a remote file
  upload <local> [remote]   upload a local file
  dns <host[:type]>         resolve a host (A, AAAA, MX, TXT, NS, CNAME, ...)
  exec <command>            run a shell command on the server
  !<command>                shortcut for exec
  help                      show this help
  exit                      leave the shell
`)
}

func (c *client) execRequest(op, path, data string, offset int) (map[string][]byte, error) {
	info := map[string][]byte{
		"CMD":  []byte(execCmd),
		"OP":   []byte(op),
		"MARK": []byte(randMark()),
	}
	if path != "" {
		info["PATH"] = []byte(path)
	}
	if data != "" {
		info["DATA"] = []byte(data)
	}
	if offset > 0 {
		info["OFFSET"] = []byte(strconv.Itoa(offset))
	}
	return c.control(info, execTimeout)
}

func execResult(rinfo map[string][]byte, err error) (string, string, int, error) {
	if err != nil {
		return "", "", 0, err
	}
	if string(rinfo["STATUS"]) != "OK" {
		return "", "", 0, fmt.Errorf("%s", responseErrorText(rinfo))
	}
	exit := 0
	if value := rinfo["EXIT"]; len(value) > 0 {
		exit, _ = strconv.Atoi(string(value))
	}
	return string(rinfo["DATA"]), string(rinfo["ERROR"]), exit, nil
}

func printShellOutput(out, errOut string, exit int) {
	if out != "" {
		fmt.Print(out)
		if !strings.HasSuffix(out, "\n") {
			fmt.Println()
		}
	}
	if errOut != "" {
		fmt.Fprint(os.Stderr, errOut)
		if !strings.HasSuffix(errOut, "\n") {
			fmt.Fprintln(os.Stderr)
		}
	}
	if exit != 0 {
		fmt.Fprintf(os.Stderr, "[exit %d]\n", exit)
	}
}

func (c *client) shellLs(args []string) {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}
	out, errOut, exit, err := execResult(c.execRequest(execOpLs, path, "", 0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ls: %v\n", err)
		return
	}
	printShellOutput(out, errOut, exit)
}

func (c *client) shellCat(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: cat <path>")
		return
	}
	path := args[0]
	offset := 0
	for {
		out, errOut, exit, err := execResult(c.execRequest(execOpCat, path, "", offset))
		if err != nil {
			fmt.Fprintf(os.Stderr, "cat: %v\n", err)
			return
		}
		if out == "" {
			if errOut != "" {
				fmt.Fprint(os.Stderr, errOut)
			}
			if exit != 0 {
				fmt.Fprintf(os.Stderr, "[exit %d]\n", exit)
			}
			return
		}
		fmt.Print(out)
		offset += len(out)
		if len(out) < execChunkSize {
			return
		}
	}
}

func (c *client) shellDownload(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: download <remote> [local]")
		return
	}
	remote := args[0]
	local := filepath.Base(remote)
	if len(args) > 1 {
		local = args[1]
	}
	file, err := os.Create(local)
	if err != nil {
		fmt.Fprintf(os.Stderr, "download: %v\n", err)
		return
	}
	defer file.Close()
	offset := 0
	for {
		out, errOut, _, err := execResult(c.execRequest(execOpDownload, remote, "", offset))
		if err != nil {
			fmt.Fprintf(os.Stderr, "download: %v\n", err)
			return
		}
		if out == "" {
			if errOut != "" {
				fmt.Fprint(os.Stderr, errOut)
			}
			break
		}
		if _, err := file.WriteString(out); err != nil {
			fmt.Fprintf(os.Stderr, "download: %v\n", err)
			return
		}
		offset += len(out)
		fmt.Fprintf(os.Stderr, "\r[download] %s: %d bytes", remote, offset)
		if len(out) < execChunkSize {
			break
		}
	}
	fmt.Fprintf(os.Stderr, "\n[download] saved %d bytes to %s\n", offset, local)
}

func (c *client) shellUpload(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: upload <local> [remote]")
		return
	}
	local := args[0]
	remote := filepath.Base(local)
	if len(args) > 1 {
		remote = args[1]
	}
	file, err := os.Open(local)
	if err != nil {
		fmt.Fprintf(os.Stderr, "upload: %v\n", err)
		return
	}
	defer file.Close()

	offset := 0
	wrote := false
	buf := make([]byte, execChunkSize)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			wrote = true
			if err := c.uploadChunk(remote, buf[:n], offset); err != nil {
				fmt.Fprintf(os.Stderr, "upload: %v\n", err)
				return
			}
			offset += n
			fmt.Fprintf(os.Stderr, "\r[upload] %s: %d bytes", remote, offset)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "upload: %v\n", readErr)
			return
		}
	}
	if !wrote {
		if err := c.uploadChunk(remote, nil, 0); err != nil {
			fmt.Fprintf(os.Stderr, "upload: %v\n", err)
			return
		}
	}
	fmt.Fprintf(os.Stderr, "\n[upload] saved %d bytes to %s\n", offset, remote)
}

func (c *client) uploadChunk(remote string, data []byte, offset int) error {
	_, _, _, err := execResult(c.execRequest(execOpUpload, remote, string(data), offset))
	return err
}

func (c *client) shellDNS(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: dns <host[:type]>")
		return
	}
	out, errOut, exit, err := execResult(c.execRequest(execOpDNS, args[0], "", 0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "dns: %v\n", err)
		return
	}
	printShellOutput(out, errOut, exit)
}

func (c *client) shellExec(cmd string) {
	if strings.TrimSpace(cmd) == "" {
		fmt.Fprintln(os.Stderr, "usage: exec <command>")
		return
	}
	out, errOut, exit, err := execResult(c.execRequest(execOpShell, "", cmd, 0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec: %v\n", err)
		return
	}
	printShellOutput(out, errOut, exit)
}

func splitShellArgs(line string) []string {
	var args []string
	var field strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	flush := func() {
		if field.Len() > 0 {
			args = append(args, field.String())
			field.Reset()
		}
	}
	for _, r := range line {
		if escaped {
			field.WriteRune(r)
			escaped = false
			continue
		}
		switch {
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t') && !inSingle && !inDouble:
			flush()
		default:
			field.WriteRune(r)
		}
	}
	flush()
	return args
}
