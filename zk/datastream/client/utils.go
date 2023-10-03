package client

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

func writeFullUint64ToConn(conn net.Conn, value uint64) error {
	buffer := make([]byte, 8)
	binary.BigEndian.PutUint64(buffer, uint64(value))

	if conn == nil {
		return errors.New("error nil connection")
	}

	_, err := conn.Write(buffer)
	if err != nil {
		return fmt.Errorf("%s Error sending to server: %v", conn.RemoteAddr().String(), err)
	}

	return nil
}

func readBuffer(conn net.Conn, n uint32) ([]byte, error) {
	buffer := make([]byte, n)
	rbc, err := io.ReadFull(conn, buffer)
	if err != nil {
		return []byte{}, parseIoReadError(err)
	}

	if uint32(rbc) != n {
		return []byte{}, fmt.Errorf("expected to read %d bytes, bute got %d", n, rbc)
	}

	return buffer, nil
}

func parseIoReadError(err error) error {
	if err == io.EOF {
		return errors.New("server close connection")
	} else {
		return fmt.Errorf("error reading from server: %v", err)
	}
}
