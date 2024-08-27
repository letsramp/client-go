/*
 * Copyright Skyramp Authors 2024
 */
package types

import (
	"encoding/json"
	"errors"
)

// returns whether test executed successfully
// return false and error messsage if failed
func (r *TestStatusResponse) Executed() (bool, error) {
	if len(r.Results) > 0 {
		return r.Status == "finished" && r.Results[1].Executed, errors.New(r.Error)
	}
	return false, errors.New(r.Error)
}

func (r *TestStatusResponse) String() string {
	if r == nil {
		return ""
	}

	res, _ := json.Marshal(r)
	return string(res)
}

func (r *TestStatusResponse) StringIndent() string {
	if r == nil {
		return ""
	}

	res, _ := json.MarshalIndent(r, "", "  ")
	return string(res)
}
