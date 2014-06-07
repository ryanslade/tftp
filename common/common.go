package common

type OpCode uint16

const (
	OpRRQ   OpCode = 1
	OpWRQ   OpCode = 2
	OpDATA  OpCode = 3
	OpACK   OpCode = 4
	OpERROR OpCode = 5
)

var OpCodeNames = map[OpCode]string{
	OpRRQ:   "RRQ",
	OpWRQ:   "WRQ",
	OpDATA:  "DATA",
	OpACK:   "ACK",
	OpERROR: "ERROR",
}

func (o OpCode) String() string {
	return OpCodeNames[o]
}

type RequestPacket struct {
	OpCode   OpCode
	Filename string
	Mode     string
}
