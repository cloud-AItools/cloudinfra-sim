// srcdigest는 구현 식별자를 출력한다. 빌드: go build -ldflags "-X cloudinfra-sim/internal/sim.BuildCommit=$(go run ./cmd/srcdigest)"
package main

import (
	"fmt"
	"os"

	"cloudinfra-sim/internal/buildinfo"
)

func main() {
	d, err := buildinfo.SourceDigest(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(d)
}
