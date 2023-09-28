package client

import "fmt"

const (
	// Commands
	CmdUnknown Command = 0
	CmdStart   Command = 1
	CmdStop    Command = 2
	CmdHeader  Command = 3
)

func (c *StreamClient) sendHeaderCmd() error {
	err := c.sendCommand(CmdHeader)
	if err != nil {
		return fmt.Errorf("%s %v", c.id, err)
	}

	return nil
}

func (c *StreamClient) sendStartCmd(from uint64) error {
	err := c.sendCommand(CmdStart)
	if err != nil {
		return err
	}

	// Send starting/from entry number
	if err := writeFullUint64ToConn(c.conn, from); err != nil {
		return err
	}

	return nil
}

func (c *StreamClient) sendCommand(cmd Command) error {
	// Send command
	if err := writeFullUint64ToConn(c.conn, uint64(cmd)); err != nil {
		return fmt.Errorf("%s %v", c.id, err)
	}

	// Send stream type
	if err := writeFullUint64ToConn(c.conn, uint64(c.streamType)); err != nil {
		return fmt.Errorf("%s %v", c.id, err)
	}

	return nil
}
