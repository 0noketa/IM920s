package IM920s

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/tarm/serial"
)

type Config struct {
	Id       int
	Name     string
	Baud     int
	TextMode bool
}
type Port struct {
	Port   *serial.Port
	Config Config
}
type Packet struct {
	sender int
	rssi   int
	data   []byte
}
type TimeInfo struct {
	H, M, S, Ms int
	Sync        bool
}

type Error struct {
	msg string
}

type Baud int

const (
	BAUD_1200   = 0
	BAUD_2400   = 1
	BAUD_4800   = 2
	BAUD_9600   = 3
	BAUD_19200  = 4
	BAUD_38400  = 5
	BAUD_57600  = 6
	BAUD_115200 = 7
	BAUD_230400 = 8
	BAUD_460800 = 9
)

func (err Error) Error() string {
	return err.msg
}
func newError(msg string) Error {
	return Error{msg: msg}
}

func Open(config Config) (*Port, error) {
	port, err := serial.OpenPort(&serial.Config{Name: config.Name, Baud: config.Baud})
	if err != nil {
		return nil, err
	}

	return &Port{Port: port, Config: config}, nil
}

func (port *Port) Write(bs []byte) (int, error) {
	if len(bs) > 32 {
		bs = bs[:32]
	}
	return port.Port.Write(bs)
}
func (port *Port) WriteLine(bs []byte) (int, error) {
	if len(bs) > 30 {
		bs = append(bs[:30], []byte("\r\n")...)
	}
	return port.Port.Write(bs)
}

func (port *Port) ReadLine(bs []byte) (int, error) {
	var b [1]byte

	i := 0
	for ; i < len(bs); i++ {
		_, err := port.Port.Read(b[:])
		if err != nil {
			return i, err
		}

		if b[0] == 0 {
			break
		}

		if b[0] == 13 {
			_, err = port.Port.Read(b[:])
			if err != nil {
				return i, err
			}

			break
		}

		bs[i] = b[0]
	}

	return i, nil
}
func (port *Port) ReadStringLine() (string, error) {
	var bs [32]byte

	n, err := port.ReadLine(bs[:])
	if err != nil {
		return "", err
	}

	return string(bs[:n]), nil
}

func (port *Port) ReadParams(params map[string][]byte) error {
	port.WriteLine([]byte("RPRM"))
	port.Flush()

	if params == nil {
		params = make(map[string][]byte)
	}

	for {
		bs := make([]byte, 64)
		n, err := port.ReadLine(bs)
		if err != nil {
			return err
		}
		bs = bs[:n]

		i := bytes.Index(bs, []byte{byte(':')})
		if i == -1 {
			params["_"] = bs
			break
		}

		key := string(bs[:i])
		val := bs[i+1:]
		params[key] = val
	}

	port.Config.TextMode = strings.Contains(string(params["_"]), "ECIO")

	id, err := strconv.Atoi(string(params["STNN"]))
	if err != nil {
		return err
	}

	port.Config.Id = id
	return nil
}

func (port *Port) SetTextMode(textMode bool) error {
	if port.Config.TextMode == textMode {
		return nil
	}

	if textMode {
		_, err := port.WriteLine([]byte("ECIO"))
		if err != nil {
			return err
		}
	} else {
		_, err := port.WriteLine([]byte("DCIO"))
		if err != nil {
			return err
		}
	}

	err := port.CheckResult()
	if err != nil {
		return err
	}

	port.Config.TextMode = textMode
	return nil
}

func (port *Port) Close() error {
	return port.Port.Close()
}

func (port *Port) Flush() error {
	return port.Port.Flush()
}
func (port *Port) CheckResult() error {
	err := port.Flush()
	if err != nil {
		return err
	}

	s, err := port.ReadStringLine()
	if err != nil {
		return err
	}
	if s != "OK" {
		return newError("unknown bahaviour")
	}

	return nil
}

func (port *Port) SendTo(addr int, data []byte) error {
	if len(data) <= 32 {
		size := len(data)

		var data2 []byte
		if port.Config.TextMode {
			data2 = data
		} else {
			data2 = make([]byte, size*2)
			for i, b := range data {
				data3 := []byte(fmt.Sprintf("%02x", b))
				data2[i*2] = data3[0]
				data2[i*2+1] = data3[1]
			}
		}

		tx := append([]byte("TXDU "), []byte(fmt.Sprintf("%04x", addr))...)
		_, err := port.WriteLine(append(tx, data2...))
		if err != nil {
			return err
		}

		return port.CheckResult()
	} else {
		// send data(size>32)
		i := 0
		for ; i < len(data)-8; i += 8 {
			err := port.SendTo(addr, data[i:i+8])
			if err != nil {
				return err
			}
		}

		return port.SendTo(addr, data[i:])
	}
}

func (port *Port) Broadcast(data []byte) error {
	size := len(data)
	if size <= 32 {
		var data2 []byte
		if port.Config.TextMode {
			data2 = data
		} else {
			data2 = make([]byte, size)
			for i, b := range data {
				data3 := []byte(fmt.Sprintf("%02x", b))
				data2[i*2] = data3[0]
				data2[i*2+1] = data3[1]
			}
		}

		var tx []byte
		if size <= 8 {
			tx = []byte("TXDA ")
		} else {
			tx = []byte("TXDT ")
		}
		tx = append(tx, data2...)
		port.Write(append(tx, []byte("\r\n")...))

		return port.CheckResult()
	} else {
		i := 0
		for ; i < len(data)-8; i += 8 {
			err := port.Broadcast(data[i : i+8])
			if err != nil {
				return err
			}

			err = port.CheckResult()
			if err != nil {
				return err
			}
		}

		err := port.Broadcast(data[i:])
		if err != nil {
			return err
		}

		err = port.CheckResult()
		if err != nil {
			return err
		}

		return nil
	}
}

func (port *Port) ReadByte() (byte, error) {
	var bs [1]byte
	_, err := port.Port.Read(bs[:])

	if err != nil {
		return 0, err
	}
	return bs[0], nil
}

func (port *Port) ReceivePacket() (*Packet, error) {
	// """-> (sender, RSSI, data)"""

	for {
		var bs [32]byte
		n, err := port.ReadLine(bs[:])
		if err != nil {
			return nil, err
		}

		if n >= 11 {
			data := bs[11:]
			m := n - 11

			// # bug? 2nd or later packets have space at the first of payload.
			if data[0] == 32 {
				data = data[1:]
			}

			var data2 []byte
			if port.Config.TextMode {
				data2 = data
			} else {
				data2 = make([]byte, m/2)
				for i := 0; i < len(data); i += 2 {
					var b byte
					_, err := fmt.Sscanf(string(data[i:i+1]), "%02x", &b)
					if err != nil {
						return nil, err
					}
					data2[i/2] = b
				}
			}

			var sender int
			_, err := fmt.Sscan(string(bs[3:7]), "%x", &sender)
			if err != nil {
				return nil, err
			}
			var rssi int
			fmt.Sscan(string(bs[8:10]), "%x", &rssi)
			if err != nil {
				return nil, err
			}

			return &Packet{sender, -rssi, data2}, nil
		}
	}
}

// def change_baudrate(self, baudrate: int) -> bool:
//         pass

//     def change_rssi(self, int) -> bool:
//         pass

//     def sleep(self, int) -> bool:
//         pass

//     def wake(self):
//         pass

func (port *Port) SetBaud(baud Baud) error {
	tx := []byte(fmt.Sprintf("SBRT %v", baud))
	_, err := port.WriteLine(tx)
	if err != nil {
		return err
	}

	return port.CheckResult()
}

func (port *Port) SetTime(h, m, s int) error {
	tx := []byte(fmt.Sprintf("STCK %02v %02v %02v", h, m, s))
	_, err := port.WriteLine(tx)
	if err != nil {
		return err
	}

	return port.CheckResult()
}

func (port *Port) GetTime() (*TimeInfo, error) {
	_, err := port.WriteLine([]byte("RDCK"))
	if err != nil {
		return nil, err
	}

	var dst [32]byte
	n, err := port.ReadLine(dst[:])
	if err != nil {
		return nil, err
	}

	var h, m, s, ms int
	var c rune
	n, err = fmt.Sscanf(string(dst[:n]), "%02d:%02d:%02d.%d %c", &h, &m, &s, &ms, &c)
	if err != nil && n == 5 {
		return nil, err
	}

	err = port.CheckResult()
	if err != nil {
		return nil, err
	}

	return &TimeInfo{h, m, s, ms, c == 'Y'}, nil
}

func main() {
	port, err := Open(Config{Name: "/dev/ttyUSB0", Baud: 19200})
	if err != nil {
		return
	}
	err = port.Broadcast([]byte("hello"))
	if err != nil {
		return
	}
	err = port.Close()
	if err != nil {
		return
	}
}
