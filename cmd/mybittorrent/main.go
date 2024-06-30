package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"unicode"
	// bencode "github.com/jackpal/bencode-go" // Available if you need it!
)

// Example:
// - 5:hello -> hello
// - 10:hello12345 -> hello12345
func decodeBencode(bencodedString string) (interface{}, error) {
	switch {
	case unicode.IsDigit(rune(bencodedString[0])):
		return decodeString(bencodedString)
	case rune(bencodedString[0]) == 'i':
		return decodeNumber(bencodedString)
	default:
		return "", fmt.Errorf("only strings are supported at the moment")
	}
}

func decodeString(bencodedString string) (string, error) {
	// find length of string
	lengthStr := 0
	i := 0
	for i < len(bencodedString) && bencodedString[i] >= '0' && bencodedString[i] <= '9' {
		lengthStr = lengthStr*10 + int(bencodedString[i]) - '0'
		i++
	}

	if i == len(bencodedString) || bencodedString[i] != ':' {
		return "", fmt.Errorf("bad formatted string")
	}

	i++

	if i+lengthStr > len(bencodedString) {
		return "", fmt.Errorf("index %d out of bounds for string length %d", i+lengthStr, len(bencodedString))
	}

	return bencodedString[i : i+lengthStr], nil
}

func decodeNumber(bencodedString string) (int, error) {
	res, err := strconv.Atoi(bencodedString[1 : len(bencodedString)-1])
	if err != nil {
		panic(err)
	}

	return res, nil
}

func main() {
	command := os.Args[1]

	if command == "decode" {
		bencodedValue := os.Args[2]

		decoded, err := decodeBencode(bencodedValue)
		if err != nil {
			fmt.Println(err)
			return
		}

		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}
