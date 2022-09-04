# Golang dynamic struct

Package dynamic struct provides possibility to dynamically, in runtime,
extend or merge existing defined structs or to provide completely new struct.

Main features:
* Building completely new struct in runtime
* Extending existing struct in runtime
* Merging multiple structs in runtime
* Adding new fields into struct
* Removing existing fields from struct
* Modifying fields' types and tags
* Easy reading of dynamic structs
* Mapping dynamic struct with set values to existing struct
* Make slices and maps of dynamic structs

## Benchmarks

Environment:
* MacBook Pro (13-inch, Early 2015), 2,7 GHz Intel Core i5
* go version go1.11 darwin/amd64

```
goos: darwin
goarch: amd64
pkg: github.com/ompluscator/dynamic-struct
BenchmarkClassicWay_NewInstance-4                 2000000000     0.34 ns/op
BenchmarkNewStruct_NewInstance-4                    10000000      141 ns/op
BenchmarkNewStruct_NewInstance_Parallel-4           20000000     89.6 ns/op
BenchmarkExtendStruct_NewInstance-4                 10000000      135 ns/op
BenchmarkExtendStruct_NewInstance_Parallel-4        20000000     89.5 ns/op
BenchmarkMergeStructs_NewInstance-4                 10000000      140 ns/op
BenchmarkMergeStructs_NewInstance_Parallel-4        20000000     94.3 ns/op
```

## Add new struct
```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/wind-c/comqtt/server/internal/dtruct"
)

func main() {
	instance := dtruct.NewStruct().
		AddField("Integer", 0, `json:"int"`).
		AddField("Text", "", `json:"someText"`).
		AddField("Float", 0.0, `json:"double"`).
		AddField("Boolean", false, "").
		AddField("Slice", []int{}, "").
		AddField("Anonymous", "", `json:"-"`).
		Build().
		New()

	data := []byte(`
{
    "int": 123,
    "someText": "example",
    "double": 123.45,
    "Boolean": true,
    "Slice": [1, 2, 3],
    "Anonymous": "avoid to read"
}
`)

	err := json.Unmarshal(data, &instance)
	if err != nil {
		log.Fatal(err)
	}

	data, err = json.Marshal(instance)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(data))
	// Out:
	// {"int":123,"someText":"example","double":123.45,"Boolean":true,"Slice":[1,2,3]}
}
```

## Extend existing struct
```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/wind-c/comqtt/server/internal/dtruct"
)

type Data struct {
	Integer int `json:"int"`
}

func main() {
	instance := dtruct.ExtendStruct(Data{}).
		AddField("Text", "", `json:"someText"`).
		AddField("Float", 0.0, `json:"double"`).
		AddField("Boolean", false, "").
		AddField("Slice", []int{}, "").
		AddField("Anonymous", "", `json:"-"`).
		Build().
		New()

	data := []byte(`
{
    "int": 123,
    "someText": "example",
    "double": 123.45,
    "Boolean": true,
    "Slice": [1, 2, 3],
    "Anonymous": "avoid to read"
}
`)

	err := json.Unmarshal(data, &instance)
	if err != nil {
		log.Fatal(err)
	}

	data, err = json.Marshal(instance)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(data))
	// Out:
	// {"int":123,"someText":"example","double":123.45,"Boolean":true,"Slice":[1,2,3]}
}
```

## Merge existing structs
```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/wind-c/comqtt/server/internal/dtruct"
)

type DataOne struct {
	Integer int     `json:"int"`
	Text    string  `json:"someText"`
	Float   float64 `json:"double"`
}

type DataTwo struct {
	Boolean bool
	Slice []int
	Anonymous string `json:"-"`
}

func main() {
	instance := dtruct.MergeStructs(DataOne{}, DataTwo{}).
		Build().
		New()

	data := []byte(`
{
"int": 123,
"someText": "example",
"double": 123.45,
"Boolean": true,
"Slice": [1, 2, 3],
"Anonymous": "avoid to read"
}
`)

	err := json.Unmarshal(data, &instance)
	if err != nil {
		log.Fatal(err)
	}

	data, err = json.Marshal(instance)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(data))
	// Out:
	// {"int":123,"someText":"example","double":123.45,"Boolean":true,"Slice":[1,2,3]}
}
```

## Read dynamic struct

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/wind-c/comqtt/server/internal/dtruct"
)

type DataOne struct {
	Integer int     `json:"int"`
	Text    string  `json:"someText"`
	Float   float64 `json:"double"`
}

type DataTwo struct {
	Boolean bool
	Slice []int
	Anonymous string `json:"-"`
}

func main() {
	instance := dtruct.MergeStructs(DataOne{}, DataTwo{}).
		Build().
		New()

	data := []byte(`
{
"int": 123,
"someText": "example",
"double": 123.45,
"Boolean": true,
"Slice": [1, 2, 3],
"Anonymous": "avoid to read"
}
`)

	err := json.Unmarshal(data, &instance)
	if err != nil {
		log.Fatal(err)
	}

	reader := dynamicstruct.NewReader(instance)

	fmt.Println("Integer", reader.GetField("Integer").Int())
	fmt.Println("Text", reader.GetField("Text").String())
	fmt.Println("Float", reader.GetField("Float").Float64())
	fmt.Println("Boolean", reader.GetField("Boolean").Bool())
	fmt.Println("Slice", reader.GetField("Slice").Interface().([]int))
	fmt.Println("Anonymous", reader.GetField("Anonymous").String())

	var dataOne DataOne
	err = reader.ToStruct(&dataOne)
	fmt.Println(err, dataOne)

	var dataTwo DataTwo
	err = reader.ToStruct(&dataTwo)
	fmt.Println(err, dataTwo)
	// Out:
	// Integer 123
	// Text example
	// Float 123.45
	// Boolean true
	// Slice [1 2 3]
	// Anonymous
	// <nil> {123 example 123.45}
	// <nil> {true [1 2 3] }
}
```

## Make a slice of dynamic struct

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/wind-c/comqtt/server/internal/dtruct"
)

type Data struct {
	Integer   int     `json:"int"`
	Text      string  `json:"someText"`
	Float     float64 `json:"double"`
	Boolean   bool
	Slice     []int
	Anonymous string `json:"-"`
}

func main() {
	definition := dtruct.ExtendStruct(Data{}).Build()

	slice := definition.NewSliceOfStructs()

	data := []byte(`
[
	{
		"int": 123,
		"someText": "example",
		"double": 123.45,
		"Boolean": true,
		"Slice": [1, 2, 3],
		"Anonymous": "avoid to read"
	}
]
`)

	err := json.Unmarshal(data, &slice)
	if err != nil {
		log.Fatal(err)
	}

	data, err = json.Marshal(slice)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(data))
	// Out:
	// [{"Boolean":true,"Slice":[1,2,3],"int":123,"someText":"example","double":123.45}]

	reader := dynamicstruct.NewReader(slice)
	readersSlice := reader.ToSliceOfReaders()
	for k, v := range readersSlice {
		var value Data
		err := v.ToStruct(&value)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(k, value)
	}
	// Out:
	// 0 {123 example 123.45 true [1 2 3] }
}

```

## Make a map of dynamic struct

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/wind-c/comqtt/server/internal/dtruct"
)

type Data struct {
	Integer   int     `json:"int"`
	Text      string  `json:"someText"`
	Float     float64 `json:"double"`
	Boolean   bool
	Slice     []int
	Anonymous string `json:"-"`
}

func main() {
	definition := dtruct.ExtendStruct(Data{}).Build()

	mapWithStringKey := definition.NewMapOfStructs("")

	data := []byte(`
{
	"element": {
		"int": 123,
		"someText": "example",
		"double": 123.45,
		"Boolean": true,
		"Slice": [1, 2, 3],
		"Anonymous": "avoid to read"
	}
}
`)

	err := json.Unmarshal(data, &mapWithStringKey)
	if err != nil {
		log.Fatal(err)
	}

	data, err = json.Marshal(mapWithStringKey)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(data))
	// Out:
	// {"element":{"int":123,"someText":"example","double":123.45,"Boolean":true,"Slice":[1,2,3]}}

	reader := dtruct.NewReader(mapWithStringKey)
	readersMap := reader.ToMapReaderOfReaders()
	for k, v := range readersMap {
		var value Data
		err := v.ToStruct(&value)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(k, value)
	}
	// Out:
	// element {123 example 123.45 true [1 2 3] }
}

```