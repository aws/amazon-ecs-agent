//The codec package defines how to read and write data from the wire
package codec

import (
	"io"
)

//A Codec defines how to read and write data from the wire
type Codec interface {
	RoundTrip(*Request, io.ReadWriter) error
}
type ShapeRef struct {
	ShapeName string
}
type Request struct {
	Service   ShapeRef
	Operation ShapeRef
	Input     interface{}
	Output    interface{}
}
