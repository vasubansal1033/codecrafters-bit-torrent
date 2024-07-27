package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	Complete    int    `json:"complete"`
	Incomplete  int    `json:"incomplete"`
	Interval    int    `json:"interval"`
	MinInterval int    `json:"min interval"`
	Peers       string `json:"peers"`
}

type Peer struct {
	IP   net.IP
	Port uint16
}

type PeerMessage struct {
	messageLength uint32
	messageId     uint8
	payload       PeerMessagePayload
}

type PeerMessagePayload struct {
	index  uint32
	offset uint32
	length uint32
}

type HandshakeMessage struct {
	Length        int
	Protocol      string
	ReservedBytes [8]byte
	InfoHash      string
	PeerId        string
}

const (
	BIT_FIELD_MESSAGE_ID  = 5
	INTERESTED_MESSAGE_ID = 2
	UNCHOKE_MESSAGE_ID    = 1
	BLOCK_SIZE            = 16 * 1024
	REQUEST_MESSAGE_ID    = 6
	PIECE_MESSAGE_ID      = 7
)

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

		finalUrl := getPeerDiscoveryUrl(
			string(hexDecodedHash),
			"00112233445566778899",
			"6881",
			"0",
			"0",
			parsedTorrentFile.info.length,
			"1",
			parsedTorrentFile.trackerUrl,
		)

		peers := performPeerDiscovery(finalUrl)

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

		handshakeMessage := HandshakeMessage{
			Length:   19,
			Protocol: "BitTorrent protocol",
			InfoHash: string(hexDecodedHash),
			PeerId:   "00112233445566778899",
		}

		peerAddress := os.Args[3]
		conn, err := net.Dial("tcp", peerAddress)
		if err != nil {
			panic(err)
		}

		defer conn.Close()

		handShakeResponse := performHandshake(conn, handshakeMessage.getBytes())

		fmt.Printf("Peer ID: %s\n", handShakeResponse.PeerId)

	case "download_piece":
		var torrentFile, outputPath string
		if os.Args[2] == "-o" {
			torrentFile = os.Args[4]
			outputPath = os.Args[3]
		} else {
			torrentFile = os.Args[2]
			outputPath = "."
		}

		data, err := os.ReadFile(torrentFile)
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

		handshakeMessage := HandshakeMessage{
			Length:   19,
			Protocol: "BitTorrent protocol",
			InfoHash: string(hexDecodedHash),
			PeerId:   "00112233445566778899",
		}

		finalUrl := getPeerDiscoveryUrl(
			string(hexDecodedHash),
			"00112233445566778899",
			"6881",
			"0",
			"0",
			parsedTorrentFile.info.length,
			"1",
			parsedTorrentFile.trackerUrl,
		)

		peers := performPeerDiscovery(finalUrl)

		peerAddress := fmt.Sprintf("%s:%d", peers[0].IP.String(), peers[0].Port)
		conn, err := net.Dial("tcp", peerAddress)
		if err != nil {
			panic(err)
		}

		defer conn.Close()

		_ = performHandshake(conn, handshakeMessage.getBytes())

		pieceIndex, _ := strconv.Atoi(os.Args[5])
		downloadedPiece := downloadPiece(conn, parsedTorrentFile, pieceIndex)

		file, err := os.Create(outputPath)
		if err != nil {
			panic(err)
		}

		defer file.Close()

		_, err = file.Write(downloadedPiece)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Piece downloaded to %s.\n", outputPath)
	case "download":
		var torrentFile, outputPath string
		if os.Args[2] == "-o" {
			torrentFile = os.Args[4]
			outputPath = os.Args[3]
		} else {
			torrentFile = os.Args[2]
			outputPath = "."
		}

		data, err := os.ReadFile(torrentFile)
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

		handshakeMessage := HandshakeMessage{
			Length:   19,
			Protocol: "BitTorrent protocol",
			InfoHash: string(hexDecodedHash),
			PeerId:   "00112233445566778899",
		}

		finalUrl := getPeerDiscoveryUrl(
			string(hexDecodedHash),
			"00112233445566778899",
			"6881",
			"0",
			"0",
			parsedTorrentFile.info.length,
			"1",
			parsedTorrentFile.trackerUrl,
		)

		peers := performPeerDiscovery(finalUrl)

		peerAddress := fmt.Sprintf("%s:%d", peers[1].IP.String(), peers[1].Port)

		file, err := os.Create(outputPath)
		if err != nil {
			panic(err)
		}

		defer file.Close()

		numPieces := len(parsedTorrentFile.info.pieces)
		for pieceIndex := 0; pieceIndex < numPieces; pieceIndex++ {
			fmt.Printf("Downloading piece %d\n", pieceIndex)

			fmt.Println("Performing handshake")
			conn, err := net.Dial("tcp", peerAddress)
			if err != nil {
				panic(err)
			}

			_ = performHandshake(conn, handshakeMessage.getBytes())

			downloadedPiece := downloadPiece(conn, parsedTorrentFile, pieceIndex)
			_, err = file.Write(downloadedPiece)
			if err != nil {
				panic(err)
			}

			fmt.Printf("Piece downloaded to %s.\n", outputPath)
			fmt.Println("Closing connection.")
			conn.Close()
		}

	default:
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}

func downloadPiece(conn net.Conn, parsedTorrentFile ParsedTorrentFile, index int) []byte {
	// wait for bitfield message (id = 5)
	peerMessage := PeerMessage{}

	peerMessageLengthBytes := make([]byte, 4)

	_, err := conn.Read(peerMessageLengthBytes)
	if err != nil {
		panic(err)
	}

	peerMessage.messageLength = binary.BigEndian.Uint32(peerMessageLengthBytes)

	peerMessageIdBytes := make([]byte, peerMessage.messageLength)
	_, err = conn.Read(peerMessageIdBytes)
	if err != nil {
		panic(err)
	}

	peerMessage.messageId = peerMessageIdBytes[0]
	if peerMessage.messageId != BIT_FIELD_MESSAGE_ID {
		panic(fmt.Errorf("expected bitfield message"))
	}

	// send an interested message (id=2)
	_, err = conn.Write([]byte{0, 0, 0, 1, INTERESTED_MESSAGE_ID})
	if err != nil {
		panic(err)
	}

	// wait for unchoke message(id=1)
	unchokeMessage := PeerMessage{}
	unchokeMessageLengthBytes := make([]byte, 4)

	_, err = conn.Read(unchokeMessageLengthBytes)
	if err != nil {
		panic(err)
	}

	unchokeMessage.messageLength = binary.BigEndian.Uint32(unchokeMessageLengthBytes)

	unchokeMessageIdBytes := make([]byte, unchokeMessage.messageLength)

	_, err = conn.Read(unchokeMessageIdBytes)
	if err != nil {
		panic(err)
	}

	unchokeMessage.messageId = unchokeMessageIdBytes[0]

	if unchokeMessage.messageId != UNCHOKE_MESSAGE_ID {
		panic(fmt.Errorf("expected unchoke message"))
	}

	// download piece
	fileLength := parsedTorrentFile.info.length
	pieceLength := parsedTorrentFile.info.pieceLength

	pieceCnt := int(math.Ceil(float64(fileLength) / float64(pieceLength)))
	if index == pieceCnt-1 {
		pieceLength = fileLength % pieceLength
	}

	blockCnt := int(math.Ceil(float64(pieceLength) / float64(BLOCK_SIZE)))
	fmt.Printf("File Length: %d, Piece Length: %d, Piece Count: %d, Block Size: %d, Block Count: %d\n", fileLength, pieceLength, pieceCnt, BLOCK_SIZE, blockCnt)

	data := []byte{}
	for i := 0; i < blockCnt; i++ {
		blockLength := BLOCK_SIZE
		if i == blockCnt-1 {
			blockLength = pieceLength - ((blockCnt - 1) * BLOCK_SIZE)
		}

		peerMessage := PeerMessage{
			messageLength: 13,
			messageId:     REQUEST_MESSAGE_ID,
			payload: PeerMessagePayload{
				index:  uint32(index),
				offset: uint32(i * BLOCK_SIZE),
				length: uint32(blockLength),
			},
		}

		var buff bytes.Buffer
		binary.Write(&buff, binary.BigEndian, peerMessage)

		_, err := conn.Write(buff.Bytes())
		if err != nil {
			panic(err)
		}

		fmt.Println("Sent request message", peerMessage)

		// wait for piece message
		pieceMessage := PeerMessage{}
		pieceMessageLengthBytes := make([]byte, 4)
		_, err = conn.Read(pieceMessageLengthBytes)
		if err != nil {
			if err == io.EOF {
				break
			}

			panic(err)
		}

		pieceMessage.messageLength = binary.BigEndian.Uint32(pieceMessageLengthBytes)

		payloadMessageBytes := make([]byte, pieceMessage.messageLength)
		_, err = io.ReadFull(conn, payloadMessageBytes)
		if err != nil {
			panic(err)
		}

		pieceMessage.messageId = payloadMessageBytes[0]
		if pieceMessage.messageId != PIECE_MESSAGE_ID {
			panic(fmt.Errorf("expected piece message, received %d", pieceMessage.messageId))
		}

		// pieceMessage.payload.index = payloadMessageBytes[1:1]
		downloadedPieceIndex := binary.BigEndian.Uint32(payloadMessageBytes[1:5])
		downloadedPieceOffset := binary.BigEndian.Uint32(payloadMessageBytes[5:9])

		fmt.Println(downloadedPieceIndex, downloadedPieceOffset)
		data = append(data, payloadMessageBytes[9:]...)
	}

	downloadedDataHash := fmt.Sprintf("%x", sha1.Sum(data))

	if downloadedDataHash != parsedTorrentFile.info.pieces[index] {
		fmt.Println(downloadedDataHash, parsedTorrentFile.infoHash)
		panic(fmt.Errorf("downloaded data hash doesn't match with info hash in torrent file"))
	}

	return data
}

func performHandshake(
	conn net.Conn,
	handshakeMessage []byte,
) HandshakeMessage {
	_, err := conn.Write(handshakeMessage)
	if err != nil {
		panic(err)
	}

	buff := make([]byte, 68)
	_, err = conn.Read(buff)
	if err != nil {
		panic(err)
	}

	protocolLength := int(buff[0])
	handShakeResponse := HandshakeMessage{
		Length:   protocolLength,
		Protocol: string(buff[1 : 1+protocolLength]),
		InfoHash: fmt.Sprintf("%x", buff[1+protocolLength:48]),
		PeerId:   fmt.Sprintf("%x", buff[48:68]),
	}

	return handShakeResponse
}

func (h *HandshakeMessage) getBytes() []byte {
	handshakeMessage := []byte{}
	handshakeMessage = append(handshakeMessage, byte(h.Length))
	handshakeMessage = append(handshakeMessage, []byte(h.Protocol)...)
	handshakeMessage = append(handshakeMessage, make([]byte, 8)...)
	handshakeMessage = append(handshakeMessage, h.InfoHash...)
	handshakeMessage = append(handshakeMessage, []byte(h.PeerId)...)

	return handshakeMessage
}

func performPeerDiscovery(finalUrl string) []Peer {
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

	trackerResponse := TrackerResponse{
		Complete:    m["complete"].(int),
		Incomplete:  m["incomplete"].(int),
		Interval:    m["interval"].(int),
		MinInterval: m["min interval"].(int),
		Peers:       m["peers"].(string),
	}

	peers, err := parsePeers(trackerResponse.Peers)
	if err != nil {
		panic(err)
	}

	return peers
}

func getPeerDiscoveryUrl(
	hexDecodedHash string,
	peerId string, port string,
	uploaded string,
	downloaded string,
	infoLength int,
	compact string,
	trackerUrl string,
) string {
	params := url.Values{}
	params.Add("info_hash", string(hexDecodedHash))
	params.Add("peer_id", peerId)
	params.Add("port", port)
	params.Add("uploaded", uploaded)
	params.Add("downloaded", downloaded)
	params.Add("left", fmt.Sprintf("%v", infoLength))
	params.Add("compact", compact)

	finalUrl := fmt.Sprintf("%s?%s", trackerUrl, params.Encode())

	return finalUrl
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
