package swarmdkg

func NewReadChans(numOfNodes int) []chan []byte {
	chans := make([]chan []byte, numOfNodes)
	for i := range chans {
		chans[i] = make(chan []byte, 10)
	}
	return chans
}

func NewStreamerMock(readChans []chan []byte, nodeNum int) *streamerMock {
	return &streamerMock{
		readChans: readChans,
		nodeNum:   nodeNum,
	}
}

type streamerMock struct {
	readChans []chan []byte
	nodeNum   int
}

func (m *streamerMock) Broadcast(msg []byte) {
	for i := range m.readChans {
		m.readChans[i] <- msg
	}
}

func (m *streamerMock) Read() chan []byte {
	//fmt.Println(m.nodeNum, " returned chan")
	return m.readChans[m.nodeNum]
}
