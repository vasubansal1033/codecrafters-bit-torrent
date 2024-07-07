package main

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"unicode"

	bencode "github.com/jackpal/bencode-go" // Available if you need it!
)

// Example:
// - 5:hello -> hello
// - 10:hello12345 -> hello12345

func decodeBencode(bencodedString string, st int) (x interface{}, i int, err error) {
	switch {
	case rune(bencodedString[st]) == 'd':
		return decodeDict(bencodedString, st)
	case rune(bencodedString[st]) == 'l':
		return decodeList(bencodedString, st)
	case unicode.IsDigit(rune(bencodedString[st])):
		return decodeString(bencodedString, st)
	case rune(bencodedString[st]) == 'i':
		return decodeNumber(bencodedString, st)
	default:
		return "", st, fmt.Errorf("unexpected value: %q", bencodedString[i])
	}
}

func decodeDict(bencodedString string, st int) (m map[string]interface{}, i int, err error) {
	i = st
	i++

	m = make(map[string]interface{})
	for {
		if i >= len(bencodedString) {
			return nil, st, fmt.Errorf("bad formatted dictionary")
		}

		if bencodedString[i] == 'e' {
			i++
			break
		}

		pairSt := i
		var key, val interface{}

		key, i, err = decodeBencode(bencodedString, i)
		if err != nil {
			return nil, pairSt, err
		}

		k, ok := key.(string)
		if !ok {
			return nil, pairSt, fmt.Errorf("key is not a string")
		}

		val, i, err = decodeBencode(bencodedString, i)
		if err != nil {
			return nil, i, err
		}

		m[k] = val
	}

	return m, i, nil
}

func decodeList(bencodedString string, st int) (list []interface{}, i int, err error) {
	i = st
	i++

	list = make([]interface{}, 0)
	for {
		if i >= len(bencodedString) {
			return nil, st, fmt.Errorf("bad formatted list")
		}

		if bencodedString[i] == 'e' {
			i++
			break
		}

		var li interface{}
		li, i, err = decodeBencode(bencodedString, i)

		if err != nil {
			return nil, i, err
		}

		list = append(list, li)
	}

	return list, i, err
}

func decodeString(bencodedString string, st int) (string, int, error) {
	// find length of string
	lengthStr := 0
	i := st
	for i < len(bencodedString) && bencodedString[i] >= '0' && bencodedString[i] <= '9' {
		lengthStr = lengthStr*10 + int(bencodedString[i]) - '0'
		i++
	}

	if i == len(bencodedString) || bencodedString[i] != ':' {
		return "", st, fmt.Errorf("bad formatted string")
	}

	i++

	if i+lengthStr > len(bencodedString) {
		return "", st, fmt.Errorf("index %d out of bounds for string length %d", i+lengthStr, len(bencodedString))
	}

	return bencodedString[i : i+lengthStr], i + lengthStr, nil
}

func decodeNumber(bencodedString string, st int) (int, int, error) {
	i := st
	i++

	if len(bencodedString) == i {
		return 0, st, fmt.Errorf("bad formatted number")
	}

	isNegative := false
	if bencodedString[i] == '-' {
		isNegative = true
		i++
	}

	x := 0
	for i < len(bencodedString) && (bencodedString[i] >= '0' && bencodedString[i] <= '9') {
		x = x*10 + int(bencodedString[i]) - '0'
		i++
	}

	if i == len(bencodedString) || bencodedString[i] != 'e' {
		return 0, st, fmt.Errorf("bad formatted number")
	}

	if isNegative {
		x *= -1
	}

	i++

	return x, i, nil
}

func main() {
	command := os.Args[1]

	switch command {
	case "decode":
		bencodedValue := os.Args[2]

		decoded, _, err := decodeBencode(bencodedValue, 0)
		if err != nil {
			fmt.Println(err)
			return
		}

		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	case "info":
		data, err := os.ReadFile(os.Args[2])
		if err != nil {
			fmt.Printf("error: read file: %v\n", err)
			os.Exit(1)
		}

		decoded, _, err := decodeDict(string(data), 0)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Tracker URL: %v\n", decoded["announce"])

		info, ok := decoded["info"].(map[string]interface{})
		if info == nil || !ok {
			fmt.Printf("no info section\n")
			return
		}

		fmt.Printf("Length: %v\n", info["length"])

		h := sha1.New()
		if err := bencode.Marshal(h, info); err != nil {
			panic(err)
		}

		fmt.Printf("Info Hash: %x\n", h.Sum(nil))
	default:
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}

}
