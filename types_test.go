// Copyright 2014 Ryan Rogers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nhlgc

import (
	"encoding/xml"
	"testing"
	"time"
)

func TestOptionalUint64_UnmarshalXML(t *testing.T) {
	type TestStruct struct {
		Present  OptionalUint64 `xml:"present"`
		Optional OptionalUint64 `xml:"optional"`
	}
	testXML := []byte(`
	<?xml version="1.0" encoding="UTF-8"?>
	<result>
		<present>1</present>
		<optional></optional>
	</result>
	`)

	var result TestStruct
	if err := xml.Unmarshal(testXML, &result); err != nil {
		t.Fatalf("Expected Unmarshal to succeed, received error '%v'.", err)
	}
	if result.Present != 1 {
		t.Errorf("Expected element 'present' to be 1, received '%v'.", result.Present)
	}
	if result.Optional != 0 {
		t.Errorf("Expected element 'optional' to be 0, received '%v'.", result.Optional)
	}
}

func TestGameTimeGMT_UnmarshalXML(t *testing.T) {
	type TestStruct struct {
		Date GameTimeGMT `xml:"date"`
	}
	testXML := []byte(`
	<?xml version="1.0" encoding="UTF-8"?>
	<result>
		<date>2014-11-08T00:00:00.000</date>
	</result>
	`)
	expected, err := time.Parse(time.RFC3339Nano, "2014-11-08T00:00:00.000Z")
	if err != nil {
		t.Fatalf("Expected Parse to succeed, received error '%v'.", err)
	}

	var result TestStruct
	if err := xml.Unmarshal(testXML, &result); err != nil {
		t.Fatalf("Expected Unmarshal to succeed, received error '%v'.", err)
	}
	if !time.Time(result.Date).Equal(expected) {
		t.Errorf("Expected date '%v', received '%v'.", expected, result.Date.String())
	}
}
