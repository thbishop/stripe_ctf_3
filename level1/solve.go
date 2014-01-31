package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func worker(id int, jobs <-chan int) {
	for j := range jobs {
		h := hash(body(strconv.Itoa(j)))
		if h < diff {
			print("\nWe mined a gitcoin!!!!\ncounter == " + strconv.Itoa(j) + " & sha1 == " + h + "\n")
			os.Exit(0)
		}
	}
}


func difficulty() string {
	data, err := ioutil.ReadFile("./difficulty.txt")
	if err != nil {
		log.Fatal(err)
	}
	return string(data)
}

func GitTree() string {
	args := []string{"write-tree"}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		log.Fatal(err)
	}
	return strings.Trim(string(out), "\n")
}

func GitPrevious() string {
	args := []string{"rev-parse", "HEAD"}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		log.Fatal(err)
	}
	return strings.Trim(string(out), "\n")
}

func Now() string {
	args := []string{"+%s"}
	out, err := exec.Command("date", args...).Output()
	if err != nil {
		log.Fatal(err)
	}
	return strings.Trim(string(out), "\n")
}

func hash(data string) string {
	h := sha1.New()
	io.WriteString(h, data)
	return hex.EncodeToString(h.Sum(nil))
}

func body(count string) string {
	data := "tree " + tree + "\n" +
		   "parent " + previous + "\n" +
		   "author CTF user <me@example.com> " + now + " +0000" + "\n" +
		   "committer CTF user <me@example.com> " + now + " +0000" + "\n\n" +
		   "Give me a Gitcoin" + "\n\n" +
		   count

	return data
}

var diff = difficulty()
var previous = GitPrevious()
var tree = GitTree()
var now = Now()

func main() {
	nCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(nCPU)
	fmt.Println("Number of CPUs: ", nCPU)
	jobs := make(chan int, 1000000)

	for w := 1; w <= 80; w++ {
		go worker(w, jobs)
	}

	counter := 1
	for {
		jobs <- counter
		counter += counter
	}
}
