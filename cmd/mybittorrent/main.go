package main

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"unicode"

	bencode "github.com/jackpal/bencode-go" // Available if you need it!
)

// Example:
// - 5:hello -> hello
// - 10:hello12345 -> hello12345

type ParsedTorrentFile struct {
	trackerUrl string
	infoHash   string
	info       TorrentInfo
}

type TorrentInfo struct {
	length      int
	pieceLength int
	pieces      []string
}

type TrackerResponse struct {
	interval int    `json:"interval"`
	peers    string `json:"peers"`
}

type Peer struct {
	IP   net.IP
	Port uint16
}

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

		parsedTorrentFile, err := parseTorrentFile(data)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Tracker URL: %v\n", parsedTorrentFile.trackerUrl)
		fmt.Printf("Length: %v\n", parsedTorrentFile.info.length)
		fmt.Printf("Info Hash: %v\n", parsedTorrentFile.infoHash)
		fmt.Printf("Piece Length: %v\n", parsedTorrentFile.info.pieceLength)
		fmt.Println("Piece Hashes:")
		pieces := parsedTorrentFile.info.pieces
		if err != nil {
			panic(err)
		}

		for _, piece := range pieces {
			fmt.Printf("%v\n", piece)
		}
	case "peers":
		data, err := os.ReadFile(os.Args[2])
		if err != nil {
			fmt.Printf("error: read file: %v\n", err)
			os.Exit(1)
		}

		parsedTorrentFile, err := parseTorrentFile(data)
		if err != nil {
			panic(err)
		}

		hexDecodedHash, err := hex.DecodeString(parsedTorrentFile.infoHash)
		if err != nil {
			panic(err)
		}

		params := url.Values{}
		params.Add("info_hash", string(hexDecodedHash))
		params.Add("peer_id", "00112233445566778899")
		params.Add("port", "6881")
		params.Add("uploaded", "0")
		params.Add("downloaded", "0")
		params.Add("left", fmt.Sprintf("%v", parsedTorrentFile.info.length))
		params.Add("compact", "1")

		finalUrl := fmt.Sprintf("%s?%s", parsedTorrentFile.trackerUrl, params.Encode())
		res, err := http.Get(finalUrl)
		if err != nil {
			panic(err)
		}

		defer res.Body.Close()
		resBytes, err := io.ReadAll(res.Body)
		if err != nil {
			panic(err)
		}

		m, _, err := decodeDict(string(resBytes), 0)
		if err != nil {
			panic(err)
		}

		peers, err := parsePeers(m["peers"].(string))
		if err != nil {
			panic(err)
		}

		for _, peer := range peers {
			fmt.Printf("%v:%v\n", peer.IP, peer.Port)
		}
	case "handshake":
		data, err := os.ReadFile(os.Args[2])
		if err != nil {
			fmt.Printf("error: read file: %v\n", err)
			os.Exit(1)
		}

		parsedTorrentFile, err := parseTorrentFile(data)
		if err != nil {
			panic(err)
		}

		hexDecodedHash, err := hex.DecodeString(parsedTorrentFile.infoHash)
		if err != nil {
			panic(err)
		}

		peerAddress := os.Args[3]
		conn, err := net.Dial("tcp", peerAddress)
		if err != nil {
			panic(err)
		}

		defer conn.Close()

		handshakeMessage := []byte{}
		handshakeMessage = append(handshakeMessage, byte(19))
		handshakeMessage = append(handshakeMessage, []byte("BitTorrent protocol")...)
		handshakeMessage = append(handshakeMessage, make([]byte, 8)...)
		handshakeMessage = append(handshakeMessage, hexDecodedHash...)
		handshakeMessage = append(handshakeMessage, []byte("00112233445566778899")...)

		_, err = conn.Write(handshakeMessage)
		if err != nil {
			panic(err)
		}

		buff := make([]byte, 68)
		_, err = conn.Read(buff)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Peer ID: %x\n", string(buff[48:]))
	default:
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}

}

func parsePeers(peers string) ([]Peer, error) {
	const peerSize = 6
	if len(peers)%peerSize != 0 {
		return nil, fmt.Errorf("peers string has incorrect length")
	}

	var result []Peer
	for i := 0; i < len(peers); i += peerSize {
		ip := net.IP(peers[i : i+4])
		port := binary.BigEndian.Uint16([]byte(peers[i+4 : i+6]))
		result = append(result, Peer{IP: ip, Port: port})
	}

	return result, nil
}

func parseTorrentFile(data []byte) (ParsedTorrentFile, error) {
	decoded, _, err := decodeDict(string(data), 0)
	if err != nil {
		panic(err)
	}

	info, ok := decoded["info"].(map[string]interface{})
	if info == nil || !ok {
		return ParsedTorrentFile{}, fmt.Errorf("no info section")
	}

	h := sha1.New()
	if err := bencode.Marshal(h, info); err != nil {
		panic(err)
	}

	pieces, err := getPieces(info["pieces"])
	if err != nil {
		panic(err)
	}

	parsedTorrent := ParsedTorrentFile{
		trackerUrl: decoded["announce"].(string),
		infoHash:   fmt.Sprintf("%x", h.Sum(nil)),
		info: TorrentInfo{
			length:      info["length"].(int),
			pieceLength: info["piece length"].(int),
			pieces:      pieces,
		},
	}

	return parsedTorrent, nil
}

func getPieces(pieceI interface{}) (pieces []string, err error) {
	pieceHash, ok := pieceI.(string)
	if !ok {
		return []string{}, fmt.Errorf("error while converting pieceHash to string")
	}

	if len(pieceHash)%20 != 0 {
		return []string{}, fmt.Errorf("invalid pieces hash")
	}

	i := 0
	for i < len(pieceHash) {
		pieces = append(pieces, fmt.Sprintf("%x", pieceHash[i:i+20]))
		i += 20
	}

	return pieces, nil
}
