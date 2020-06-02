// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var testFileName string

// var _ = BeforeSuite(func() {
// 	var err error
// 	testFileName, err = CreateTempLabelMap()
// 	Expect(err).ToNot(HaveOccurred())

// 	m := make(map[string]interface{})
// 	content, err := ioutil.ReadFile(testFileName)
// 	if err != nil {
// 		fmt.Printf("failed to open testFileName file: %s", err)
// 	}
// 	if err := json.Unmarshal(content, &m); err != nil {
// 		fmt.Printf("failed to Unmarshal testFileName file: %s", err)
// 	}
// 	fmt.Printf("########################################TEST FILE: %+v\n", m)
// })

var _ = AfterSuite(func() {
	os.Remove(testFileName)
})

func TestLoki(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Loki Suite")
}
