package request

import (
	"fmt"

	"github.com/taybart/log"
)

func RunFile(filename string) error {
	requests, err := parseFile(filename)
	if err != nil {
		return err
	}
	for _, req := range requests {
		res, err := req.Do()
		if err != nil {
			log.Errorln(err)
		}
		if res != "" {
			fmt.Println(res)
		}
	}
	return nil
}

func RunLabel(filename string, label string) error {
	requests, err := parseFile(filename)
	if err != nil {
		return err
	}
	for _, req := range requests {
		if req.Label == label {
			res, err := req.Do()
			if err != nil {
				return err
			}
			if res != "" {
				fmt.Println(res)
			}
			return nil
		}
	}

	return fmt.Errorf("request label not found")
}

func RunBlock(filename string, block int) error {
	requests, err := parseFile(filename)
	if err != nil {
		return err
	}
	res, err := requests[block].Do()
	if err != nil {
		return err
	}
	if res != "" {
		fmt.Println(res)
	}
	return nil
}
