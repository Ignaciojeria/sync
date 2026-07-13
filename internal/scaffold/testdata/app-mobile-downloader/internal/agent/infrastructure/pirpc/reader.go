package pirpc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
)

func readJSONL(r io.Reader, fn func(json.RawMessage) error) error {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\r\n")
			if len(bytes.TrimSpace(line)) > 0 {
				msg := make(json.RawMessage, len(line))
				copy(msg, line)
				if callErr := fn(msg); callErr != nil {
					return callErr
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
