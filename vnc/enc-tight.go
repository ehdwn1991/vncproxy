package vnc

import (
	"errors"
	"fmt"
	"io"
)

var TightMinToCompress int = 12

const (
	TightExplicitFilter = 0x04
	TightFill           = 0x08
	TightJpeg           = 0x09
	TightMaxSubencoding = 0x09
	TightFilterCopy     = 0x00
	TightFilterPalette  = 0x01
	TightFilterGradient = 0x02
)

type TightEncoding struct {
	output io.Writer
	logger Logger
}

func (t *TightEncoding) SetOutput(output io.Writer) {
	t.output = output
}

func (*TightEncoding) Type() int32 {
	return 7
}

// func ReadAndRecBytes(conn io.Reader, rec io.Writer, count int) ([]byte, error) {
// 	buf, err := readBytes(conn, count)
// 	rec.Write(buf)
// 	return buf, err
// }
// func ReadAndRecUint8(conn io.Reader, rec io.Writer) (uint8, error) {
// 	myUint, err := readUint8(conn)
// 	buf := make([]byte, 1)
// 	buf[0] = byte(myUint) // cast int8 to byte
// 	rec.Write(buf)
// 	return myUint, err
// }

// func ReadAndRecUint16(conn io.Reader, rec io.Writer) (uint16, error) {
// 	myUint, err := readUint16(conn)
// 	buf := make([]byte, 2)
// 	//buf[0] = byte(myUint) // cast int8 to byte
// 	//var i int16 = 41
// 	//b := make([]byte, 2)
// 	binary.LittleEndian.PutUint16(buf, uint16(myUint))

// 	rec.Write(buf)
// 	return myUint, err
// }

func calcTightBytePerPixel(pf PixelFormat) int {
	bytesPerPixel := int(pf.BPP / 8)

	var bytesPerPixelTight int
	if 24 == pf.Depth && 32 == pf.BPP {
		bytesPerPixelTight = 3
	} else {
		bytesPerPixelTight = bytesPerPixel
	}
	return bytesPerPixelTight
}

func (t *TightEncoding) Read(conn *ClientConn, rect *Rectangle, reader io.Reader) (Encoding, error) {
	bytesPixel := calcTightBytePerPixel(conn.PixelFormat)

	//conn := &DataSource{conn: conn.c, PixelFormat: conn.PixelFormat}

	//var subencoding uint8
	subencoding, err := conn.readUint8()
	if err != nil {
		fmt.Printf("error in handling tight encoding: %v\n", err)
		return nil, err
	}
	fmt.Printf("bytesPixel= %d, subencoding= %d\n", bytesPixel, subencoding)
	// if err := binary.Read(conn.c, binary.BigEndian, &subencoding); err != nil {
	// 	return t, err
	// }

	//move it to position (remove zlib flush commands)
	compType := subencoding >> 4 & 0x0F
	// for stream_id := 0; stream_id < 4; stream_id++ {
	// 	//   if ((comp_ctl & 1) != 0 && tightInflaters[stream_id] != null) {
	// 	//     tightInflaters[stream_id] = null;
	// 	//   }
	// 	subencoding >>= 1
	// }

	fmt.Printf("afterSHL:%d\n", compType)
	switch compType {
	case TightFill:
		fmt.Printf("reading fill size=%d\n", bytesPixel)
		//read color
		conn.readBytes(int(bytesPixel))
		return t, nil
	case TightJpeg:
		if conn.PixelFormat.BPP == 8 {
			return nil, errors.New("Tight encoding: JPEG is not supported in 8 bpp mode")
		}

		len, err := conn.readCompactLen()
		if err != nil {
			return nil, err
		}
		fmt.Printf("reading jpeg size=%d\n", len)
		conn.readBytes(len)
		return t, nil
	default:

		if compType > TightJpeg {
			fmt.Println("Compression control byte is incorrect!")
		}

		handleTightFilters(subencoding, conn, rect, reader)
		return t, nil
	}
}

func handleTightFilters(subencoding uint8, conn *ClientConn, rect *Rectangle, reader io.Reader) {
	var FILTER_ID_MASK uint8 = 0x40
	//var STREAM_ID_MASK uint8 = 0x30

	//decoderId := (subencoding & STREAM_ID_MASK) >> 4
	var filterid uint8
	var err error

	if (subencoding & FILTER_ID_MASK) > 0 { // filter byte presence
		filterid, err = conn.readUint8()
		if err != nil {
			fmt.Printf("error in handling tight encoding, reading filterid: %v\n", err)
			return
		}
		fmt.Printf("read filter: %d\n", filterid)
	}

	//var numColors uint8
	bytesPixel := calcTightBytePerPixel(conn.PixelFormat)

	fmt.Printf("filter: %d\n", filterid)
	// if rfb.rec != null {
	// 	rfb.rec.writeByte(filter_id)
	// }
	lengthCurrentbpp := int(bytesPixel) * int(rect.Width) * int(rect.Height)

	switch filterid {
	case TightFilterPalette: //PALETTE_FILTER

		colorCount, err := conn.readUint8()
		paletteSize := colorCount + 1 // add one more
		fmt.Printf("----PALETTE_FILTER: paletteSize=%d bytesPixel=%d\n", paletteSize, bytesPixel)
		//complete palette
		conn.readBytes(int(paletteSize) * bytesPixel)

		var dataLength int
		if paletteSize == 2 {
			dataLength = int(rect.Height) * ((int(rect.Width) + 7) / 8)
		} else {
			dataLength = int(rect.Width * rect.Height)
		}
		_, err = readTightData(conn, dataLength)
		if err != nil {
			fmt.Printf("error in handling tight encoding, Reading Palette: %v\n", err)
			return
		}
	case TightFilterGradient: //GRADIENT_FILTER
		fmt.Printf("----GRADIENT_FILTER: bytesPixel=%d\n", bytesPixel)
		//useGradient = true
		fmt.Printf("usegrad: %d\n", filterid)
		readTightData(conn, lengthCurrentbpp)
	case TightFilterCopy: //BASIC_FILTER
		fmt.Printf("----BASIC_FILTER: bytesPixel=%d\n", bytesPixel)
		readTightData(conn, lengthCurrentbpp)
	default:
		fmt.Printf("Bad tight filter id: %d\n", filterid)
		return
	}

	////////////

	// if numColors == 0 && bytesPixel == 4 {
	// 	rowSize1 *= 3
	// }
	// rowSize := (int(rect.Width)*bitsPixel + 7) / 8
	// dataSize := int(rect.Height) * rowSize

	// dataSize1 := rect.Height * rowSize1
	// fmt.Printf("datasize: %d, origDatasize: %d", dataSize, dataSize1)
	// // Read, optionally uncompress and decode data.
	// if int(dataSize1) < TightMinToCompress {
	// 	// Data size is small - not compressed with zlib.
	// 	if numColors != 0 {
	// 		// Indexed colors.
	// 		//indexedData := make([]byte, dataSize)
	// 		readBytes(conn.c, int(dataSize1))
	// 		//readFully(indexedData);
	// 		// if (rfb.rec != null) {
	// 		//   rfb.rec.write(indexedData);
	// 		// }
	// 		// if (numColors == 2) {
	// 		//   // Two colors.
	// 		//   if (bytesPixel == 1) {
	// 		//     decodeMonoData(x, y, w, h, indexedData, palette8);
	// 		//   } else {
	// 		//     decodeMonoData(x, y, w, h, indexedData, palette24);
	// 		//   }
	// 		// } else {
	// 		//   // 3..255 colors (assuming bytesPixel == 4).
	// 		//   int i = 0;
	// 		//   for (int dy = y; dy < y + h; dy++) {
	// 		//     for (int dx = x; dx < x + w; dx++) {
	// 		//       pixels24[dy * rfb.framebufferWidth + dx] = palette24[indexedData[i++] & 0xFF];
	// 		//     }
	// 		//   }
	// 		// }
	// 	} else if useGradient {
	// 		// "Gradient"-processed data
	// 		//buf := make ( []byte,w * h * 3);
	// 		dataByteCount := int(3) * int(rect.Width) * int(rect.Height)
	// 		readBytes(conn.c, dataByteCount)
	// 		// rfb.readFully(buf);
	// 		// if (rfb.rec != null) {
	// 		//   rfb.rec.write(buf);
	// 		// }
	// 		// decodeGradientData(x, y, w, h, buf);
	// 	} else {
	// 		// Raw truecolor data.
	// 		dataByteCount := int(bytesPixel) * int(rect.Width) * int(rect.Height)
	// 		readBytes(conn.c, dataByteCount)

	// 		// if (bytesPixel == 1) {
	// 		//   for (int dy = y; dy < y + h; dy++) {

	// 		//     rfb.readFully(pixels8, dy * rfb.framebufferWidth + x, w);
	// 		//     if (rfb.rec != null) {
	// 		//       rfb.rec.write(pixels8, dy * rfb.framebufferWidth + x, w);
	// 		//     }
	// 		//   }
	// 		// } else {
	// 		//   byte[] buf = new byte[w * 3];
	// 		//   int i, offset;
	// 		//   for (int dy = y; dy < y + h; dy++) {
	// 		//     rfb.readFully(buf);
	// 		//     if (rfb.rec != null) {
	// 		//       rfb.rec.write(buf);
	// 		//     }
	// 		//     offset = dy * rfb.framebufferWidth + x;
	// 		//     for (i = 0; i < w; i++) {
	// 		//       pixels24[offset + i] = (buf[i * 3] & 0xFF) << 16 | (buf[i * 3 + 1] & 0xFF) << 8 | (buf[i * 3 + 2] & 0xFF);
	// 		//     }
	// 		//   }
	// 		// }
	// 	}
	// } else {
	// 	// Data was compressed with zlib.
	// 	zlibDataLen, err := readCompactLen(conn.c)
	// 	fmt.Printf("compactlen=%d\n", zlibDataLen)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	//byte[] zlibData = new byte[zlibDataLen];
	// 	//rfb.readFully(zlibData);
	// 	readBytes(conn.c, zlibDataLen)

	// 	//   if (rfb.rec != null) {
	// 	//     rfb.rec.write(zlibData);
	// 	//   }
	// 	//   int stream_id = comp_ctl & 0x03;
	// 	//   if (tightInflaters[stream_id] == null) {
	// 	//     tightInflaters[stream_id] = new Inflater();
	// 	//   }
	// 	//   Inflater myInflater = tightInflaters[stream_id];
	// 	//   myInflater.setInput(zlibData);
	// 	//   byte[] buf = new byte[dataSize];
	// 	//   myInflater.inflate(buf);
	// 	//   if (rfb.rec != null && !rfb.recordFromBeginning) {
	// 	//     rfb.recordCompressedData(buf);
	// 	//   }

	// 	//   if (numColors != 0) {
	// 	//     // Indexed colors.
	// 	//     if (numColors == 2) {
	// 	//       // Two colors.
	// 	//       if (bytesPixel == 1) {
	// 	//         decodeMonoData(x, y, w, h, buf, palette8);
	// 	//       } else {
	// 	//         decodeMonoData(x, y, w, h, buf, palette24);
	// 	//       }
	// 	//     } else {
	// 	//       // More than two colors (assuming bytesPixel == 4).
	// 	//       int i = 0;
	// 	//       for (int dy = y; dy < y + h; dy++) {
	// 	//         for (int dx = x; dx < x + w; dx++) {
	// 	//           pixels24[dy * rfb.framebufferWidth + dx] = palette24[buf[i++] & 0xFF];
	// 	//         }
	// 	//       }
	// 	//     }
	// 	//   } else if (useGradient) {
	// 	//     // Compressed "Gradient"-filtered data (assuming bytesPixel == 4).
	// 	//     decodeGradientData(x, y, w, h, buf);
	// 	//   } else {
	// 	//     // Compressed truecolor data.
	// 	//     if (bytesPixel == 1) {
	// 	//       int destOffset = y * rfb.framebufferWidth + x;
	// 	//       for (int dy = 0; dy < h; dy++) {
	// 	//         System.arraycopy(buf, dy * w, pixels8, destOffset, w);
	// 	//         destOffset += rfb.framebufferWidth;
	// 	//       }
	// 	//     } else {
	// 	//       int srcOffset = 0;
	// 	//       int destOffset, i;
	// 	//       for (int dy = 0; dy < h; dy++) {
	// 	//         myInflater.inflate(buf);
	// 	//         destOffset = (y + dy) * rfb.framebufferWidth + x;
	// 	//         for (i = 0; i < w; i++) {
	// 	//           pixels24[destOffset + i] = (buf[srcOffset] & 0xFF) << 16 | (buf[srcOffset + 1] & 0xFF) << 8
	// 	//               | (buf[srcOffset + 2] & 0xFF);
	// 	//           srcOffset += 3;
	// 	//         }
	// 	//       }
	// 	//     }
	// 	//   }
	// }

	return
}

func readTightData(conn *ClientConn, dataSize int) ([]byte, error) {
	if int(dataSize) < TightMinToCompress {
		return conn.readBytes(int(dataSize))
	}
	zlibDataLen, err := conn.readCompactLen()
	fmt.Printf("compactlen=%d\n", zlibDataLen)
	if err != nil {
		return nil, err
	}
	//byte[] zlibData = new byte[zlibDataLen];
	//rfb.readFully(zlibData);
	return conn.readBytes(zlibDataLen)
}
