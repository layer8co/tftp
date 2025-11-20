package tftp

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"time"
)

type Logger interface {
	Printf(format string, v ...any)
	Print(v ...any)
	Println(v ...any)
}

type TFTPServer struct {
	WriteAllowed bool
	ReadAllowed  bool
	// Where to reside the written Files
	WriteDir string
	// The payload to serve
	Payload []byte
	// The maximum retry amount in repsonse to timeout. Needs to be at
	// least 1.
	Retries uint8

	Timeout time.Duration

	Log Logger
}

func (s TFTPServer) ListenAndServe(addr string) error {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	return s.Serve(conn)
}

func (s *TFTPServer) Serve(conn net.PacketConn) error {
	if conn == nil {
		return errors.New("Nil Connection")
	}

	if s.Payload == nil {
		return errors.New("Payload is required")
	}

	if s.Retries == 0 {
		s.Retries = 10
	}

	if s.Timeout == 0 {
		s.Timeout = 4 * time.Second
	}

	var rrq ReadReq
	var wrq WriteReq
	for {
		buf := make([]byte, DatagramSize)
		_, addr, err := conn.ReadFrom(buf)
		if err != nil {
			return err
		}

		err = rrq.UnmarshalBinary(buf)
		if err == nil {
			if !s.ReadAllowed {
				data, _ := Err{Error: ErrIllegalOp, Message: "ReadReq is not allowed"}.MarshalBinary()
				_, _ = conn.WriteTo(data, addr)
			} else {
				go s.handleRead(addr.String(), rrq)
				continue
			}
		}

		err = wrq.UnmarshalBinary(buf)
		if err == nil {
			if !s.WriteAllowed {
				data, _ := Err{Error: ErrIllegalOp, Message: "WriteReq is not allowed"}.MarshalBinary()
				_, _ = conn.WriteTo(data, addr)
			} else {
				go s.handleWrite(addr.String(), wrq)
				continue
			}
		}

		s.Log.Printf("[%s] bad request: %v", addr, err)
		continue
	}
}

func (s TFTPServer) handleRead(clientAddr string, rrq ReadReq) {
	s.Log.Printf("[%s] requested read file: %s", clientAddr, rrq.Filename)

	// Using random transfer identifier for each tftp session
	conn, err := net.Dial("udp", clientAddr)
	if err != nil {
		s.Log.Printf("[%s] dial: %v", clientAddr, err)
		return
	}
	defer func() { _ = conn.Close() }()

	var (
		ackPkt  Ack
		errPkt  Err
		dataPkt = Data{Payload: bytes.NewReader(s.Payload)}
		buf     = make([]byte, DatagramSize)
	)

NEXTPACKET:
	for n := DatagramSize; n == DatagramSize; {
		data, err := dataPkt.MarshalBinary()
		if err != nil {
			s.Log.Printf("[%s] preparing data packet: %v", clientAddr, err)
			return
		}
	RETRY:
		for i := s.Retries; i > 0; i-- {
			n, err = conn.Write(data)
			if err != nil {
				s.Log.Printf("[%s] write: %v", clientAddr, err)
				return
			}
			// wait for client's Ack packet
			_ = conn.SetReadDeadline(time.Now().Add(s.Timeout))

			_, err = conn.Read(buf)
			if err != nil {
				if nErr, ok := err.(net.Error); ok && nErr.Timeout() {
					continue RETRY
				}

				s.Log.Printf("[%s] waiting for ACK: %v", clientAddr, err)
				return
			}

			switch {
			case ackPkt.UnmarshalBinary(buf) == nil:
				if uint16(ackPkt.Block) == dataPkt.Block {
					// received ACK; send next data packet
					continue NEXTPACKET
				}

			case errPkt.UnmarshalBinary(buf) == nil:
				s.Log.Printf("[%s] received error: %v",
					clientAddr, errPkt.Message)
				return
			default:
				s.Log.Printf("[%s] bad packet: %v", clientAddr, buf)
			}

		}
		s.Log.Printf("[%s] exhausted retries", clientAddr)
		return
	}
	s.Log.Printf("[%s] send %d blocks", clientAddr, dataPkt.Block)
}

func (s TFTPServer) handleWrite(clientAddr string, wrq WriteReq) {
	s.Log.Printf("[%s] Requested write file: %s", clientAddr, wrq.Filename)

	// Using random transfer identifier for each tftp session
	conn, err := net.Dial("udp", clientAddr)

	if err != nil {
		s.Log.Printf("[%s] dial: %v", clientAddr, err)
		return
	}
	defer conn.Close()

	var (
		ackPkt  Ack
		errPkt  Err
		dataPkt Data
		buf     = make([]byte, DatagramSize)
	)

	// Initial Ack packet to WRQ
	data, err := ackPkt.MarshalBinary()
	if err != nil {
		s.Log.Printf("Can not marshal the ack packet: %s", err)
		return
	}

	_, err = conn.Write(data)

	if err != nil {
		s.Log.Printf("[%s] ack: %v", clientAddr, err)
		return
	}

	file, err := os.Create(wrq.Filename)
	if err != nil {
		s.Log.Printf("[%s] CreateFile: %v", clientAddr, err)
		return
	}

	defer func() {
		err = file.Close()
		if err != nil {
			s.Log.Printf("Can not close the file: %s", err)
		}
	}()

	// Recieve datagrams until the last one comes. last datagram is always less than 516 Bytes.
	for n := DatagramSize; n == DatagramSize; {
		n, err = conn.Read(buf)
		s.Log.Println(n)
		if err != nil {
			s.Log.Printf("Error when reading from connection: %s", err)
			return
		}

		err = errPkt.UnmarshalBinary(buf)
		if err == nil {
			s.Log.Printf("[%s] received error: %v",
				clientAddr, errPkt.Message)
			return
		}

		err = dataPkt.UnmarshalBinary(buf)

		if err != nil {
			s.Log.Println(err)
			return
		}

		data, err := io.ReadAll(dataPkt.Payload)

		if err != nil {
			s.Log.Printf("Error when reading from the reader: %v", err)
			return
		}

		_, err = file.Write(data[:n-4])
		if err != nil {
			s.Log.Printf("can't write the buffer into disk: %s", err)
			return
		}

		ackPkt.Block = dataPkt.Block
		// Acknowledge the data packet
		data, err = ackPkt.MarshalBinary()
		if err != nil {
			s.Log.Printf("Can not marshal the ack packet: %s", err)
			return
		}

		_, err = conn.Write(data)

		if err != nil {
			s.Log.Printf("[%s] ack: %v", clientAddr, err)
			return
		}
	}
	// Out of the loop means we recieved every legit datagram for this connection.
	s.Log.Printf("[%s] Recieved %d blocks of data. Written to the file %s", clientAddr, ackPkt.Block, file.Name())
}
