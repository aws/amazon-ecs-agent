//Copyright (c) 2013 Tatsuo Kaniwa
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/fluent/fluent-logger-golang/fluent"
)

func main() {
	port, _ := strconv.ParseInt(os.Getenv("FLUENT_PORT"), 10, 32)
	uuid := os.Getenv("LOGGER_UUID")
	tag := "logsender-firelens-" + uuid
	logger, err := fluent.New(fluent.Config{FluentPort: int(port), FluentHost: os.Getenv("FLUENT_HOST"), Async: true})
	if err != nil {
		fmt.Println(err)
	}
	defer logger.Close()
	var data = map[string]string{
		"log": "pass"}
	e := logger.Post(tag, data)
	if e != nil {
		log.Println("Error while posting log: ", e)
	}
}
