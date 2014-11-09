// Copyright 2014 Ryan Rogers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nhlgc

import (
	"encoding/xml"
	"strconv"
	"time"
)

type OptionalUint64 uint64

func (o *OptionalUint64) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var element string
	if err := d.DecodeElement(&element, &start); err != nil {
		return err
	}
	if len(element) > 0 {
		value, err := strconv.ParseUint(element, 10, 64)
		if err != nil {
			return err
		}
		*o = OptionalUint64(value)
	}
	return nil
}

type GameTimeGMT time.Time

func (gt *GameTimeGMT) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var element string
	if err := d.DecodeElement(&element, &start); err != nil {
		return err
	}
	if len(element) > 0 {
		time, err := time.Parse(time.RFC3339Nano, element+"Z")
		if err != nil {
			return err
		}
		*gt = GameTimeGMT(time)
	}
	return nil
}

func (gt *GameTimeGMT) String() string {
	return time.Time(*gt).String()
}
