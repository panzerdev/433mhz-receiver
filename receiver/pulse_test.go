package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"log"
	"strconv"
)

func TestMatches(t *testing.T) {
	s := &Signal{
		Lengths: []int{516, 2116, 4152, 9112},
		Seq:     "0102020101020201020101020102010202020202020202010201010202010202020202020103",
	}
	p := &Protocol{
		SeqLength: 76,
		Lengths:   []int{496, 2048, 4068, 8960},
	}

	assert.Equal(t, true, matches(s, p))
}
func TestMatchesBell(t *testing.T) {
	s := &Signal{
		Lengths: []int{596, 200, 6052},
		Seq:     "01010110010110101001101010011001010101101010010112",
	}
	p := Protocols()["doorbell-old"]

	assert.Equal(t, true, matches(s, p))
}

func TestMatches_pulseSeqTooShort(t *testing.T) {
	s := &Signal{
		Lengths: []int{516, 2116, 4152, 9112},
		Seq:     "010202010102020102",
	}
	p := &Protocol{
		SeqLength: 76,
		Lengths:   []int{496, 2048, 4068, 8960},
	}

	assert.Equal(t, false, matches(s, p))
}

func TestMatches_pulseLengthDeviationTooHigh(t *testing.T) {
	s := &Signal{
		Lengths: []int{516, 2116, 4152, 9112},
		Seq:     "0102020101020201020101020102010202020202020202010201010202010202020202020103",
	}
	p := &Protocol{
		SeqLength: 76,
		Lengths:   []int{496, 2048, 2000, 8960},
	}

	assert.Equal(t, false, matches(s, p))
}

func TestMatches_numberOfPuleLengthDiffer(t *testing.T) {
	s := &Signal{
		Lengths: []int{516, 2116, 4152, 9112},
		Seq:     "0102020101020201020101020102010202020202020202010201010202010202020202020103",
	}
	p := &Protocol{
		SeqLength: 76,
		Lengths:   []int{496, 2048, 2000},
	}

	assert.Equal(t, false, matches(s, p))
}

/*
5
ACK 25.5 36.9
255
369
00000000000000000000000000010001000000000000000001010101010101010000000000000001000101010000000103

0000000000000101
0000000011111111
0000000101110001
*/

func TestConvert(t *testing.T) {
	seq := "01010101010101010101010102010201010101010101010102020202010201010101010101010102020102010101020203"
	uin, _ := strconv.ParseUint("1111111100001100",2,0)
	log.Println(uin, int16(uin))
	//0000000011110101
	bits := "10011011000"

	var m = map[string]string{
		"01": "0",
		"02": "1",
		"03": "",
	}

	mapped, err := convert(seq, m)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(mapped)

	assert.Equal(t, mapped, bits)
}

func TestPrepare(t *testing.T) {
	input := "255 2904 1388 771 11346 0 0 0 0100020002020000020002020000020002000202000200020002000200000202000200020000020002000200020002020002000002000200000002000200020002020002000200020034"

	p, _ := PreparePulse(input)

	assert.Equal(t, []int{255, 771, 1388, 2904, 11346}, p.Lengths)
	assert.Equal(t, "0300020002020000020002020000020002000202000200020002000200000202000200020000020002000200020002020002000002000200000002000200020002020002000200020014", p.Seq)
}

func TestPrepare_InvalidCharacters(t *testing.T) {
	pC := "544 4128 2100 100 140 320 808 188 01020202010202020202020202020202020101020102010101010J�G_YJ�Üxx�1��Nz�8��&[��"

	_, err := PreparePulse(pC)

	assert.Error(t, err)
}

func TestSortIndices(t *testing.T) {
	a := []int{516, 9112, 4152, 2116}

	sortedIndices := sortIndices(a)

	assert.Equal(t, sortedIndices, map[string]string{
		"0": "0",
		"1": "3",
		"2": "2",
		"3": "1",
	})
}

func TestSortSignal(t *testing.T) {
	s := &Signal{
		Lengths: []int{516, 9112, 4152, 2116},
		Seq:     "01020203",
	}

	expectedSignal := &Signal{
		Lengths: []int{516, 2116, 4152, 9112},
		Seq:     "03020201",
	}

	sortedSignal, _ := sortSignal(s)
	assert.Equal(t, expectedSignal, sortedSignal)
}

func TestSortSignal_lengthsAlreadySorted(t *testing.T) {
	s := &Signal{
		Lengths: []int{516, 2116, 4152, 9112},
		Seq:     "0102020101020201020101020102010202020202020202010201010202010202020202020103",
	}

	sortedSignal, _ := sortSignal(s)
	assert.Equal(t, s, sortedSignal)
}
