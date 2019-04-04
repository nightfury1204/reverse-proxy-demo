package filter

import "testing"

func TestResponseWriterWrapper_Write(t *testing.T) {
	r := &ResponseWriterWrapper{}
	r.Write([]byte(`{"menu": { "id": "file",
  "value": "File",
  "popup": {
    "menuitem": [
      {"value": "New", "onclick": "CreateNewDoc()"},
      {"value": "Open", "onclick": "OpenDoc()"},
      {"value": "Close", "onclick": "CloseDoc()"}
    ]
  }
}}`))
}