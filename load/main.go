package main
import (
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	rolls := []string{
		"3d6",
		"1d20",
		"1d12",
		"4d6kh3",
		"4d6kh3",
		"4d6kh3",
		"4d6kh3",
		"4d6kh3",
		"4d6kh3",
		"1d10",
	}

	for _, roll := range rolls {
		url := "http://localhost:31341?roll=" + roll
		res, err := http.Get(url)
		if err != nil {
			log.Fatal(err)
		}
		output, err := io.ReadAll(res.Body)
		if err != nil {
			return
		}
		closeErr := res.Body.Close()
		if closeErr != nil {
			log.Fatal(closeErr)
		}
		fmt.Printf("%s", output)
	}
}
