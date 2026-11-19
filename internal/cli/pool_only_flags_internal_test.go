package cli

import "testing"

// TestOptPercent: the value stays nil until given (0 is a real cap),
// prints what was given, and refuses a non-number at parse time.
func TestOptPercent(t *testing.T) {
	var v *float64
	o := optPercent{&v}
	if o.String() != "" || (optPercent{}).String() != "" || o.Type() != "percent" {
		t.Errorf("unset: %q %q", o.String(), o.Type())
	}
	if err := o.Set("0"); err != nil || v == nil || *v != 0 || o.String() != "0" {
		t.Errorf("0: %v %v %q", err, v, o.String())
	}
	if err := o.Set("62.5"); err != nil || o.String() != "62.5" {
		t.Errorf("62.5: %v %q", err, o.String())
	}
	if err := o.Set("high"); err == nil || err.Error() != `"high" is not a number` || *v != 62.5 {
		t.Errorf("high: %v", err)
	}
}
