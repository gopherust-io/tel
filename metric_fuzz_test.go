package tel

import "testing"

func FuzzAttrCacheSubject(f *testing.F) {
	f.Add("")
	f.Add("orders.created")
	f.Add("a.b.c.d.e.f")
	f.Fuzz(func(t *testing.T, subject string) {
		c := newAttrCache(64)
		_ = c.Subject(subject)
		_ = c.SubjectOpts(subject)
		_ = c.SubjectRecordOpts(subject)
	})
}
