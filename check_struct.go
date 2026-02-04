package main

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/rekognition/types"
)

func main() {
	t := reflect.TypeOf(types.CreateFaceLivenessSessionRequestSettings{})
	fmt.Println("Fields in CreateFaceLivenessSessionRequestSettings:")
	for i := 0; i < t.NumField(); i++ {
		fmt.Printf("- %s\n", t.Field(i).Name)
	}
}
