package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/abishekp123/emberkv"
)

func main() {
	dir := flag.String("dir", "", "database directory")
	cmd := flag.String("cmd", "", "set|get|delete|compact")
	key := flag.String("key", "", "key")
	value := flag.String("value", "", "value")
	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}

	db, err := emberkv.Open(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	switch *cmd {
	case "set":
		if *key == "" {
			fmt.Fprintln(os.Stderr, "-key is required for set")
			os.Exit(2)
		}
		if err := db.Set([]byte(*key), []byte(*value)); err != nil {
			fmt.Fprintf(os.Stderr, "set: %v\n", err)
			os.Exit(1)
		}
	case "get":
		if *key == "" {
			fmt.Fprintln(os.Stderr, "-key is required for get")
			os.Exit(2)
		}
		v, ok, err := db.Get([]byte(*key))
		if err != nil {
			fmt.Fprintf(os.Stderr, "get: %v\n", err)
			os.Exit(1)
		}
		if !ok {
			fmt.Println("<missing>")
			return
		}
		fmt.Println(string(v))
	case "delete":
		if *key == "" {
			fmt.Fprintln(os.Stderr, "-key is required for delete")
			os.Exit(2)
		}
		if err := db.Delete([]byte(*key)); err != nil {
			fmt.Fprintf(os.Stderr, "delete: %v\n", err)
			os.Exit(1)
		}
	case "compact":
		if err := db.Compact(); err != nil {
			fmt.Fprintf(os.Stderr, "compact: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: emberkv -dir DIR -cmd {set|get|delete|compact} [-key KEY] [-value VALUE]")
		os.Exit(2)
	}
}
