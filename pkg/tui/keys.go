package tui

import (
	"bufio"
	"io"
	"time"
)

type KeyType int

const (
	KeyNone KeyType = iota
	KeyRune
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyPageUp
	KeyPageDown
	KeyHome
	KeyEnd
	KeyEnter
	KeyEsc
	KeyBackspace
	KeyDelete
	KeyTab
	KeyCtrlC
	KeyCtrlD
)

type KeyEvent struct {
	Type KeyType
	Rune rune
}

// KeyReader reads key events from an input reader (usually os.Stdin)
type KeyReader struct {
	reader *bufio.Reader
}

func NewKeyReader(r io.Reader) *KeyReader {
	return &KeyReader{
		reader: bufio.NewReader(r),
	}
}

// ReadKey reads the next key event with ANSI escape sequence decoding
func (kr *KeyReader) ReadKey() (KeyEvent, error) {
	b, err := kr.reader.ReadByte()
	if err != nil {
		return KeyEvent{Type: KeyNone}, err
	}

	// Handle ASCII control codes
	switch b {
	case 3: // Ctrl+C
		return KeyEvent{Type: KeyCtrlC}, nil
	case 4: // Ctrl+D
		return KeyEvent{Type: KeyCtrlD}, nil
	case 9: // Tab
		return KeyEvent{Type: KeyTab}, nil
	case 10, 13: // Enter (LF or CR)
		return KeyEvent{Type: KeyEnter}, nil
	case 8, 127: // Backspace
		return KeyEvent{Type: KeyBackspace}, nil
	case 27: // Escape sequence
		// Check if more bytes are buffered
		if kr.reader.Buffered() == 0 {
			// Give a short window (25ms) for trailing escape characters to arrive
			time.Sleep(25 * time.Millisecond)
		}
		if kr.reader.Buffered() == 0 {
			return KeyEvent{Type: KeyEsc}, nil
		}

		b2, err := kr.reader.ReadByte()
		if err != nil {
			return KeyEvent{Type: KeyEsc}, nil
		}

		if b2 == '[' {
			b3, err := kr.reader.ReadByte()
			if err != nil {
				return KeyEvent{Type: KeyEsc}, nil
			}

			switch b3 {
			case 'A':
				return KeyEvent{Type: KeyUp}, nil
			case 'B':
				return KeyEvent{Type: KeyDown}, nil
			case 'C':
				return KeyEvent{Type: KeyRight}, nil
			case 'D':
				return KeyEvent{Type: KeyLeft}, nil
			case 'H':
				return KeyEvent{Type: KeyHome}, nil
			case 'F':
				return KeyEvent{Type: KeyEnd}, nil
			case '1', '2', '3', '4', '5', '6', '7', '8':
				// Extended sequences like [3~ (Delete), [5~ (PageUp), [6~ (PageDown)
				if kr.reader.Buffered() > 0 {
					b4, _ := kr.reader.ReadByte()
					if b4 == '~' {
						switch b3 {
						case '1', '7':
							return KeyEvent{Type: KeyHome}, nil
						case '3':
							return KeyEvent{Type: KeyDelete}, nil
						case '4', '8':
							return KeyEvent{Type: KeyEnd}, nil
						case '5':
							return KeyEvent{Type: KeyPageUp}, nil
						case '6':
							return KeyEvent{Type: KeyPageDown}, nil
						}
					}
				}
			}
		} else if b2 == 'O' {
			b3, _ := kr.reader.ReadByte()
			switch b3 {
			case 'H':
				return KeyEvent{Type: KeyHome}, nil
			case 'F':
				return KeyEvent{Type: KeyEnd}, nil
			}
		}

		return KeyEvent{Type: KeyEsc}, nil
	}

	// Normal printable character
	return KeyEvent{
		Type: KeyRune,
		Rune: rune(b),
	}, nil
}
