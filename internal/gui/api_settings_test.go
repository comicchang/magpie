package gui

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/settings"
)

// The Settings page sends only its own choices. What the settings keep per
// model — which models an agent is shown, and every name, level and image
// answer the user gave of a model on a page of its own — is set elsewhere, so
// a save from this page has to carry it over. This drives the real handler:
// POST /api/settings with a body naming nothing but the theme.
//
// The tables are the settings' own convention — the fields named Model* of
// type map[string]X (see settings.CarryPerModel) — found by reflection rather
// than written out here, so a table added to the settings later is covered
// without this test being changed.
func TestSettingsPageSaveCarriesEveryPerModelTable(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, ".cache"))

	fields := perModelTables(t)
	seed := settings.Load()
	for _, f := range fields {
		probe := reflect.MakeMap(f.Type)
		probe.SetMapIndex(reflect.ValueOf("probe/"+f.Name).Convert(f.Type.Key()), perModelValue(f.Type.Elem(), f.Name))
		reflect.ValueOf(&seed).Elem().FieldByIndex(f.Index).Set(probe)
	}
	if err := settings.Save(seed); err != nil {
		t.Fatal(err)
	}
	// what the settings say once written and read back, so what is compared
	// after the request is what really reached the disk, not what was set
	saved := reflect.ValueOf(settings.Load())

	rec := httptest.NewRecorder()
	Handler(nil, nil).ServeHTTP(rec, httptest.NewRequest("POST", "/api/settings", strings.NewReader(`{"theme":"dark"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("saving the theme: %d %s", rec.Code, rec.Body)
	}

	got := reflect.ValueOf(settings.Load())
	if got.FieldByName("Theme").String() != "dark" {
		t.Fatalf("the theme was saved as %q, not dark", got.FieldByName("Theme").String())
	}
	for _, f := range fields {
		was, now := saved.FieldByIndex(f.Index).Interface(), got.FieldByIndex(f.Index).Interface()
		if !reflect.DeepEqual(was, now) {
			t.Errorf("%s did not survive the Settings page's save:\n\tsaved %v\n\tnow   %v", f.Name, was, now)
		}
	}
}

// perModelTables are the settings' per-model maps: the fields named Model*
// whose type is a map[string]X, which is the convention the settings package
// keeps them by. It fails the test when the settings' own walk of that rule
// no longer agrees, so a rule changed over there is not left untested here.
func perModelTables(t *testing.T) []reflect.StructField {
	t.Helper()
	typ := reflect.TypeOf(settings.Settings{})
	var out []reflect.StructField
	for i := range typ.NumField() {
		f := typ.Field(i)
		if strings.HasPrefix(f.Name, "Model") && f.Type.Kind() == reflect.Map && f.Type.Key().Kind() == reflect.String {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		t.Fatal("the settings hold no per-model map: there is no convention here to follow")
	}
	var byPackage int
	settings.PerModelKeys(&settings.Settings{}, func(reflect.Value) { byPackage++ })
	if byPackage != len(out) {
		t.Fatalf("the settings keep %d per-model maps, this test found %d: the rule they are kept by has moved",
			byPackage, len(out))
	}
	return out
}

// perModelValue is a value of a per-model table's own type, saying which
// table it was written for, so a table that came back empty, or with another's
// entry in it, is told apart. A type with nothing said of it is left zero:
// the key alone gives the table away.
func perModelValue(typ reflect.Type, table string) reflect.Value {
	switch typ.Kind() {
	case reflect.String:
		return reflect.ValueOf("said of " + table).Convert(typ)
	case reflect.Bool:
		return reflect.ValueOf(true).Convert(typ)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(int64(len(table))).Convert(typ)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflect.ValueOf(uint64(len(table))).Convert(typ)
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(float64(len(table))).Convert(typ)
	case reflect.Slice:
		out := reflect.MakeSlice(typ, 1, 1)
		out.Index(0).Set(perModelValue(typ.Elem(), table))
		return out
	case reflect.Array:
		out := reflect.New(typ).Elem()
		out.Index(0).Set(perModelValue(typ.Elem(), table))
		return out
	case reflect.Map:
		out := reflect.MakeMap(typ)
		out.SetMapIndex(reflect.ValueOf(table).Convert(typ.Key()), perModelValue(typ.Elem(), table))
		return out
	}
	return reflect.Zero(typ)
}
