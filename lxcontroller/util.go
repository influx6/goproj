package lxcontroller

import (
	"io"
	"log"
	"strings"
	"sync"
)

func doCopy(wait *sync.WaitGroup, dst io.WriteCloser, src io.Reader) {
	wait.Add(1)
	go func() {
		if _, err := io.Copy(dst, src); err != nil {
			log.Fatal(err)
		}
		dst.Close()
		wait.Done()
	}()
}

func safeSplit(s string) []string {
	split := strings.Split(s, " ")

	var result []string
	var inquote string
	var block string
	for _, i := range split {
		if inquote == "" {
			if strings.HasPrefix(i, "'") || strings.HasPrefix(i, "\"") {
				inquote = string(i[0])
				block = strings.TrimPrefix(i, inquote) + " "
			} else {
				result = append(result, i)
			}
		} else {
			if !strings.HasSuffix(i, inquote) {
				block += i + " "
			} else {
				block += strings.TrimSuffix(i, inquote)
				inquote = ""
				result = append(result, block)
				block = ""
			}
		}
	}

	return result
}
