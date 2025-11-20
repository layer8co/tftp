# 🎿 TFTP Server
TFTP Server utility.

- [x] RFC 1350: The TFTP Protocol
- [ ] RFC 2347: TFTP Option Extension


> [!IMPORTANT]
> Currently only the `octet` transfer mode is supported.

# Usage

### As a standalone cli
```
$ ./tftp --help

 _____ _____ _____ ____  
|_   _|  ___|_   _|  _ \ 
  | | | |_    | | | |_) |
  | | |  _|   | | |  __/ 
  |_| |_|     |_| |_|    
			
  -f string
    	File to Serve
  -l string
    	Listen Address (default "127.0.0.1:69")
  -o string
    	Directory to reside the files (default ".")
  -r	Accept read request (default true)
  -w	Accept write request


$ sudo ./tftp -r -f gopher.png -l 10.0.0.220:69
``` 



### As a library
```go
package main

import (
	"io"
	"log"
	"os"
	"time"

	"github.com/Layer8Collective/tftp"
)

func main() {
	pl, _ := os.ReadFile("gopher.png")

	server := tftp.TFTPServer{
		Payload:      pl,
		WriteAllowed: true,
		ReadAllowed:  true,
		Retries:      3,
		WriteDir:     "/tmp",
		Timeout:      time.Minute,
		Log:          log.New(io.Discard, "", log.Flags()),
	}
	log.Fatal(server.ListenAndServe("10.0.0.169:69"))
}
```


