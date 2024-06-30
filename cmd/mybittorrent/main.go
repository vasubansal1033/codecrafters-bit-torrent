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
	var firstColonIndex int

	for i := 0; i < len(bencodedString); i++ {
		if bencodedString[i] == ':' {
			firstColonIndex = i
			break
		}
	}

	lengthStr := bencodedString[:firstColonIndex]

	length, err := strconv.Atoi(lengthStr)
	if err != nil {
		return "", err
	}
	if firstColonIndex+1+length > len(bencodedString) {
		return "", fmt.Errorf("index %d out of bounds as expected length is: %d but is: %d", length, firstColonIndex+1+length, len(bencodedString))
	}

	return bencodedString[firstColonIndex+1 : firstColonIndex+1+length], nil
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
