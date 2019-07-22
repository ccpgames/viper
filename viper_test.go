// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package viper

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cast"
	"github.com/stretchr/testify/assert"
)

var yamlExample = []byte(`Hacker: true
name: steve
hobbies:
- skateboarding
- snowboarding
- go
clothing:
  jacket: leather
  trousers: denim
  pants:
    size: large
age: 35
eyes : brown
beard: true
`)

var yamlExampleWithExtras = []byte(`Existing: true
Bogus: true
`)

type testUnmarshalExtra struct {
	Existing bool
}

var dotenvExample = []byte(`
TITLE_DOTENV="DotEnv Example"
TYPE_DOTENV=donut
NAME_DOTENV=Cake`)

func initConfigs() {
	Reset()
	var r io.Reader
	SetConfigType("yaml")
	r = bytes.NewReader(yamlExample)
	_ = unmarshalReader(r, v.config)

	SetConfigType("env")
	r = bytes.NewReader(dotenvExample)
	_ = unmarshalReader(r, v.config)
}

func initConfig(typ, config string) {
	Reset()
	SetConfigType(typ)
	r := strings.NewReader(config)

	if err := unmarshalReader(r, v.config); err != nil {
		panic(err)
	}
}

func initYAML() {
	initConfig("yaml", string(yamlExample))
}

func initDotEnv() {
	Reset()
	SetConfigType("env")
	r := bytes.NewReader(dotenvExample)

	_ = unmarshalReader(r, v.config)
}

// make directories for testing
func initDirs(t *testing.T) (string, string, func()) {

	var (
		testDirs = []string{`a a`, `b`, `c\c`, `D_`}
		config   = `improbable`
	)

	root, err := ioutil.TempDir("", "")

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Chdir("..")
			_ = os.RemoveAll(root)
		}
	}()

	assert.Nil(t, err)

	err = os.Chdir(root)
	assert.Nil(t, err)

	for _, dir := range testDirs {
		err = os.Mkdir(dir, 0750)
		assert.Nil(t, err)

		err = ioutil.WriteFile(
			path.Join(dir, config+".yaml"),
			[]byte("key: \"value is "+dir+"\"\n"),
			0640)
		assert.Nil(t, err)
	}

	cleanup = false
	return root, config, func() {
		_ = os.Chdir("..")
		_ = os.RemoveAll(root)
	}
}

func TestBasics(t *testing.T) {
	SetConfigFile("/tmp/config.yaml")
	filename, err := v.getConfigFile()
	assert.Equal(t, "/tmp/config.yaml", filename)
	assert.NoError(t, err)
}

func TestDefault(t *testing.T) {
	SetDefault("age", 45)
	assert.Equal(t, 45, Get("age"))

	SetDefault("clothing.jacket", "slacks")
	assert.Equal(t, "slacks", Get("clothing.jacket"))

	SetConfigType("yaml")
	err := ReadConfig(bytes.NewBuffer(yamlExample))

	assert.NoError(t, err)
	assert.Equal(t, "leather", Get("clothing.jacket"))
}

func TestUnmarshaling(t *testing.T) {
	SetConfigType("yaml")
	r := bytes.NewReader(yamlExample)

	_ = unmarshalReader(r, v.config)
	assert.True(t, InConfig("name"))
	assert.False(t, InConfig("state"))
	assert.Equal(t, "steve", Get("name"))
	assert.Equal(t, []interface{}{"skateboarding", "snowboarding", "go"}, Get("hobbies"))
	assert.Equal(t, map[string]interface{}{"jacket": "leather", "trousers": "denim", "pants": map[string]interface{}{"size": "large"}}, Get("clothing"))
	assert.Equal(t, 35, Get("age"))
}

func TestUnmarshalExact(t *testing.T) {
	vip := New()
	target := &testUnmarshalExtra{}
	vip.SetConfigType("yaml")
	r := bytes.NewReader(yamlExampleWithExtras)
	_ = vip.ReadConfig(r)
	err := vip.UnmarshalExact(target)
	if err == nil {
		t.Fatal("UnmarshalExact should error when populating a struct from a conf that contains unused fields")
	}
}

func TestOverrides(t *testing.T) {
	Set("age", 40)
	assert.Equal(t, 40, Get("age"))
}

func TestDefaultPost(t *testing.T) {
	assert.NotEqual(t, "NYC", Get("state"))
	SetDefault("state", "NYC")
	assert.Equal(t, "NYC", Get("state"))
}

func TestAliases(t *testing.T) {
	RegisterAlias("years", "age")
	assert.Equal(t, 40, Get("years"))
	Set("years", 45)
	assert.Equal(t, 45, Get("age"))
}

func TestAliasInConfigFile(t *testing.T) {
	// the config file specifies "beard".  If we make this an alias for
	// "hasbeard", we still want the old config file to work with beard.
	RegisterAlias("beard", "hasbeard")
	assert.Equal(t, true, Get("hasbeard"))
	Set("hasbeard", false)
	assert.Equal(t, false, Get("beard"))
}

func TestYML(t *testing.T) {
	initYAML()
	assert.Equal(t, "steve", Get("name"))
}

func TestDotEnv(t *testing.T) {
	initDotEnv()
	assert.Equal(t, "DotEnv Example", Get("title_dotenv"))
}

func TestEnv(t *testing.T) {
	initYAML()

	_ = BindEnv("id")
	_ = BindEnv("f", "FOOD")

	_ = os.Setenv("ID", "13")
	_ = os.Setenv("FOOD", "apple")
	_ = os.Setenv("NAME", "crunk")

	assert.Equal(t, "13", Get("id"))
	assert.Equal(t, "apple", Get("f"))
	assert.Equal(t, "steve", Get("name"))

	AutomaticEnv()

	assert.Equal(t, "crunk", Get("name"))

}

func TestEmptyEnv(t *testing.T) {
	initYAML()

	_ = BindEnv("age")  // Empty environment variable
	_ = BindEnv("name") // Bound, but not set environment variable

	os.Clearenv()

	os.Setenv("AGE", "")

	assert.Equal(t, 35, Get("age"))
	assert.Equal(t, "steve", Get("name"))
}

func TestEmptyEnv_Allowed(t *testing.T) {
	initYAML()

	AllowEmptyEnv(true)

	_ = BindEnv("age")  // Empty environment variable
	_ = BindEnv("name") // Bound, but not set environment variable

	os.Clearenv()

	os.Setenv("AGE", "")

	assert.Equal(t, "", Get("age"))
	assert.Equal(t, "steve", Get("name"))
}

func TestEnvPrefix(t *testing.T) {
	initYAML()

	SetEnvPrefix("foo") // will be uppercased automatically
	_ = BindEnv("id")
	_ = BindEnv("f", "FOOD") // not using prefix

	_ = os.Setenv("FOO_ID", "13")
	_ = os.Setenv("FOOD", "apple")
	_ = os.Setenv("FOO_NAME", "crunk")

	assert.Equal(t, "13", Get("id"))
	assert.Equal(t, "apple", Get("f"))
	assert.Equal(t, "steve", Get("name"))

	AutomaticEnv()

	assert.Equal(t, "crunk", Get("name"))
}

func TestAutoEnv(t *testing.T) {
	Reset()

	AutomaticEnv()
	os.Setenv("FOO_BAR", "13")
	assert.Equal(t, "13", Get("foo_bar"))
}

func TestAutoEnvWithPrefix(t *testing.T) {
	Reset()

	AutomaticEnv()
	SetEnvPrefix("Baz")
	os.Setenv("BAZ_BAR", "13")
	assert.Equal(t, "13", Get("bar"))
}

func TestSetEnvKeyReplacer(t *testing.T) {
	Reset()

	AutomaticEnv()
	os.Setenv("REFRESH_INTERVAL", "30s")

	replacer := strings.NewReplacer("-", "_")
	SetEnvKeyReplacer(replacer)

	assert.Equal(t, "30s", Get("refresh-interval"))
}

func TestAllKeys(t *testing.T) {
	initConfigs()

	ks := sort.StringSlice{"hacker", "name", "hobbies", "clothing.jacket", "clothing.trousers", "clothing.pants.size", "age", "eyes", "beard", "title_dotenv", "type_dotenv", "name_dotenv"}
	all := map[string]interface{}{"eyes": "brown", "clothing": map[string]interface{}{"trousers": "denim", "jacket": "leather", "pants": map[string]interface{}{"size": "large"}}, "hacker": true, "name": "steve", "beard": true, "hobbies": []interface{}{"skateboarding", "snowboarding", "go"}, "age": 35, "title_dotenv": "DotEnv Example", "type_dotenv": "donut", "name_dotenv": "Cake"}

	var allkeys sort.StringSlice = AllKeys()
	allkeys.Sort()
	ks.Sort()

	assert.Equal(t, ks, allkeys)
	assert.Equal(t, all, AllSettings())
}

func TestAllKeysWithEnv(t *testing.T) {
	v := New()

	// bind and define environment variables (including a nested one)
	_ = v.BindEnv("id")
	_ = v.BindEnv("foo.bar")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	os.Setenv("ID", "13")
	os.Setenv("FOO_BAR", "baz")

	expectedKeys := sort.StringSlice{"id", "foo.bar"}
	expectedKeys.Sort()
	keys := sort.StringSlice(v.AllKeys())
	keys.Sort()
	assert.Equal(t, expectedKeys, keys)
}

func TestAliasesOfAliases(t *testing.T) {
	Set("Title", "Checking Case")
	RegisterAlias("Foo", "Bar")
	RegisterAlias("Bar", "Title")
	assert.Equal(t, "Checking Case", Get("FOO"))
}

func TestRecursiveAliases(t *testing.T) {
	RegisterAlias("Baz", "Roo")
	RegisterAlias("Roo", "baz")
}

func TestUnmarshal(t *testing.T) {
	SetDefault("port", 1313)
	Set("name", "Steve")
	Set("duration", "1s1ms")
	Set("modes", []int{1, 2, 3})

	type config struct {
		Port     int
		Name     string
		Duration time.Duration
		Modes    []int
	}

	var C config

	err := Unmarshal(&C)
	if err != nil {
		t.Fatalf("unable to decode into struct, %v", err)
	}

	assert.Equal(
		t,
		&config{
			Name:     "Steve",
			Port:     1313,
			Duration: time.Second + time.Millisecond,
			Modes:    []int{1, 2, 3},
		},
		&C,
	)

	Set("port", 1234)
	err = Unmarshal(&C)
	if err != nil {
		t.Fatalf("unable to decode into struct, %v", err)
	}

	assert.Equal(
		t,
		&config{
			Name:     "Steve",
			Port:     1234,
			Duration: time.Second + time.Millisecond,
			Modes:    []int{1, 2, 3},
		},
		&C,
	)
}

func TestUnmarshalWithDecoderOptions(t *testing.T) {
	Set("credentials", "{\"foo\":\"bar\"}")

	opt := DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		// Custom Decode Hook Function
		func(rf reflect.Kind, rt reflect.Kind, data interface{}) (interface{}, error) {
			if rf != reflect.String || rt != reflect.Map {
				return data, nil
			}
			m := map[string]string{}
			raw := data.(string)
			if raw == "" {
				return m, nil
			}
			return m, json.Unmarshal([]byte(raw), &m)
		},
	))

	type config struct {
		Credentials map[string]string
	}

	var C config

	err := Unmarshal(&C, opt)
	if err != nil {
		t.Fatalf("unable to decode into struct, %v", err)
	}

	assert.Equal(t, &config{
		Credentials: map[string]string{"foo": "bar"},
	}, &C)
}

func TestBoundCaseSensitivity(t *testing.T) {
	assert.Equal(t, "brown", Get("eyes"))

	_ = BindEnv("eYEs", "TURTLE_EYES")
	_ = os.Setenv("TURTLE_EYES", "blue")

	assert.Equal(t, "blue", Get("eyes"))
}

func TestSizeInBytes(t *testing.T) {
	input := map[string]uint{
		"":               0,
		"b":              0,
		"12 bytes":       0,
		"200000000000gb": 0,
		"12 b":           12,
		"43 MB":          43 * (1 << 20),
		"10mb":           10 * (1 << 20),
		"1gb":            1 << 30,
	}

	for str, expected := range input {
		assert.Equal(t, expected, parseSizeInBytes(str), str)
	}
}

func TestFindsNestedKeys(t *testing.T) {
	initConfigs()

	Set("super", map[string]interface{}{
		"deep": map[string]interface{}{
			"nested": "value",
		},
	})

	expected := map[string]interface{}{
		"hobbies": []interface{}{
			"skateboarding", "snowboarding", "go",
		},
		"TITLE_DOTENV": "DotEnv Example",
		"TYPE_DOTENV":  "donut",
		"NAME_DOTENV":  "Cake",
		"eyes":         "brown",
		"age":          35,
		"name":         "steve",
		"hacker":       true,
		"clothing": map[string]interface{}{
			"jacket":   "leather",
			"trousers": "denim",
			"pants": map[string]interface{}{
				"size": "large",
			},
		},
		"clothing.jacket":     "leather",
		"clothing.pants.size": "large",
		"clothing.trousers":   "denim",
		"beard":               true,
	}

	for key, expectedValue := range expected {
		assert.Equal(t, expectedValue, v.Get(key))
	}

}

func TestReadBufConfig(t *testing.T) {
	v := New()
	v.SetConfigType("yaml")
	_ = v.ReadConfig(bytes.NewBuffer(yamlExample))
	t.Log(v.AllKeys())

	assert.True(t, v.InConfig("name"))
	assert.False(t, v.InConfig("state"))
	assert.Equal(t, "steve", v.Get("name"))
	assert.Equal(t, []interface{}{"skateboarding", "snowboarding", "go"}, v.Get("hobbies"))
	assert.Equal(t, map[string]interface{}{"jacket": "leather", "trousers": "denim", "pants": map[string]interface{}{"size": "large"}}, v.Get("clothing"))
	assert.Equal(t, 35, v.Get("age"))
}

func TestIsSet(t *testing.T) {
	v := New()
	v.SetConfigType("yaml")
	_ = v.ReadConfig(bytes.NewBuffer(yamlExample))
	assert.True(t, v.IsSet("clothing.jacket"))
	assert.False(t, v.IsSet("clothing.jackets"))
	assert.False(t, v.IsSet("helloworld"))
	v.Set("helloworld", "fubar")
	assert.True(t, v.IsSet("helloworld"))
}

func TestDirsSearch(t *testing.T) {
	root, config, cleanup := initDirs(t)
	defer cleanup()

	v := New()
	v.SetConfigName(config)
	v.SetDefault(`key`, `default`)

	entries, _ := ioutil.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() {
			v.AddConfigPath(e.Name())
		}
	}

	err := v.ReadInConfig()
	assert.Nil(t, err)

	assert.Equal(t, `value is `+path.Base(v.configPaths[0]), v.GetString(`key`))
}

func TestWrongDirsSearchNotFound(t *testing.T) {

	_, config, cleanup := initDirs(t)
	defer cleanup()

	v := New()
	v.SetConfigName(config)
	v.SetDefault(`key`, `default`)

	v.AddConfigPath(`whattayoutalkingbout`)
	v.AddConfigPath(`thispathaintthere`)

	err := v.ReadInConfig()
	assert.Equal(t, reflect.TypeOf(ConfigFileNotFoundError{"", ""}), reflect.TypeOf(err))

	// Even though config did not load and the error might have
	// been ignored by the client, the default still loads
	assert.Equal(t, `default`, v.GetString(`key`))
}

func TestWrongDirsSearchNotFoundForMerge(t *testing.T) {

	_, config, cleanup := initDirs(t)
	defer cleanup()

	v := New()
	v.SetConfigName(config)
	v.SetDefault(`key`, `default`)

	v.AddConfigPath(`whattayoutalkingbout`)
	v.AddConfigPath(`thispathaintthere`)

	err := v.MergeInConfig()
	assert.Equal(t, reflect.TypeOf(ConfigFileNotFoundError{"", ""}), reflect.TypeOf(err))

	// Even though config did not load and the error might have
	// been ignored by the client, the default still loads
	assert.Equal(t, `default`, v.GetString(`key`))
}

func TestSub(t *testing.T) {
	v := New()
	v.SetConfigType("yaml")
	_ = v.ReadConfig(bytes.NewBuffer(yamlExample))

	subv := v.Sub("clothing")
	assert.Equal(t, v.Get("clothing.pants.size"), subv.Get("pants.size"))

	subv = v.Sub("clothing.pants")
	assert.Equal(t, v.Get("clothing.pants.size"), subv.Get("size"))

	subv = v.Sub("clothing.pants.size")
	assert.Equal(t, (*Viper)(nil), subv)

	subv = v.Sub("missing.key")
	assert.Equal(t, (*Viper)(nil), subv)
}

var yamlMergeExampleTgt = []byte(`
hello:
    pop: 37890
    largenum: 765432101234567
    num2pow63: 9223372036854775808
    world:
    - us
    - uk
    - fr
    - de
`)

var yamlMergeExampleSrc = []byte(`
hello:
    pop: 45000
    largenum: 7654321001234567
    universe:
    - mw
    - ad
    ints:
    - 1
    - 2
fu: bar
`)

func TestMergeConfig(t *testing.T) {
	v := New()
	v.SetConfigType("yml")
	if err := v.ReadConfig(bytes.NewBuffer(yamlMergeExampleTgt)); err != nil {
		t.Fatal(err)
	}

	if pop := v.GetInt("hello.pop"); pop != 37890 {
		t.Fatalf("pop != 37890, = %d", pop)
	}

	if pop := v.GetInt32("hello.pop"); pop != int32(37890) {
		t.Fatalf("pop != 37890, = %d", pop)
	}

	if pop := v.GetInt64("hello.largenum"); pop != int64(765432101234567) {
		t.Fatalf("int64 largenum != 765432101234567, = %d", pop)
	}

	if pop := v.GetUint("hello.pop"); pop != 37890 {
		t.Fatalf("uint pop != 37890, = %d", pop)
	}

	if pop := v.GetUint32("hello.pop"); pop != 37890 {
		t.Fatalf("uint32 pop != 37890, = %d", pop)
	}

	if pop := v.GetUint64("hello.num2pow63"); pop != 9223372036854775808 {
		t.Fatalf("uint64 num2pow63 != 9223372036854775808, = %d", pop)
	}

	if world := v.GetStringSlice("hello.world"); len(world) != 4 {
		t.Fatalf("len(world) != 4, = %d", len(world))
	}

	if fu := v.GetString("fu"); fu != "" {
		t.Fatalf("fu != \"\", = %s", fu)
	}

	if err := v.MergeConfig(bytes.NewBuffer(yamlMergeExampleSrc)); err != nil {
		t.Fatal(err)
	}

	if pop := v.GetInt("hello.pop"); pop != 45000 {
		t.Fatalf("pop != 45000, = %d", pop)
	}

	if pop := v.GetInt32("hello.pop"); pop != int32(45000) {
		t.Fatalf("pop != 45000, = %d", pop)
	}

	if pop := v.GetInt64("hello.largenum"); pop != int64(7654321001234567) {
		t.Fatalf("int64 largenum != 7654321001234567, = %d", pop)
	}

	if world := v.GetStringSlice("hello.world"); len(world) != 4 {
		t.Fatalf("len(world) != 4, = %d", len(world))
	}

	if universe := v.GetStringSlice("hello.universe"); len(universe) != 2 {
		t.Fatalf("len(universe) != 2, = %d", len(universe))
	}

	if ints := v.GetIntSlice("hello.ints"); len(ints) != 2 {
		t.Fatalf("len(ints) != 2, = %d", len(ints))
	}

	if fu := v.GetString("fu"); fu != "bar" {
		t.Fatalf("fu != \"bar\", = %s", fu)
	}
}

func TestMergeConfigNoMerge(t *testing.T) {
	v := New()
	v.SetConfigType("yml")
	if err := v.ReadConfig(bytes.NewBuffer(yamlMergeExampleTgt)); err != nil {
		t.Fatal(err)
	}

	if pop := v.GetInt("hello.pop"); pop != 37890 {
		t.Fatalf("pop != 37890, = %d", pop)
	}

	if world := v.GetStringSlice("hello.world"); len(world) != 4 {
		t.Fatalf("len(world) != 4, = %d", len(world))
	}

	if fu := v.GetString("fu"); fu != "" {
		t.Fatalf("fu != \"\", = %s", fu)
	}

	if err := v.ReadConfig(bytes.NewBuffer(yamlMergeExampleSrc)); err != nil {
		t.Fatal(err)
	}

	if pop := v.GetInt("hello.pop"); pop != 45000 {
		t.Fatalf("pop != 45000, = %d", pop)
	}

	if world := v.GetStringSlice("hello.world"); len(world) != 0 {
		t.Fatalf("len(world) != 0, = %d", len(world))
	}

	if universe := v.GetStringSlice("hello.universe"); len(universe) != 2 {
		t.Fatalf("len(universe) != 2, = %d", len(universe))
	}

	if ints := v.GetIntSlice("hello.ints"); len(ints) != 2 {
		t.Fatalf("len(ints) != 2, = %d", len(ints))
	}

	if fu := v.GetString("fu"); fu != "bar" {
		t.Fatalf("fu != \"bar\", = %s", fu)
	}
}

func TestMergeConfigMap(t *testing.T) {
	v := New()
	v.SetConfigType("yml")
	if err := v.ReadConfig(bytes.NewBuffer(yamlMergeExampleTgt)); err != nil {
		t.Fatal(err)
	}

	assert := func(i int) {
		large := v.GetInt64("hello.largenum")
		pop := v.GetInt("hello.pop")
		if large != 765432101234567 {
			t.Fatal("Got large num:", large)
		}

		if pop != i {
			t.Fatal("Got pop:", pop)
		}
	}

	assert(37890)

	update := map[string]interface{}{
		"Hello": map[string]interface{}{
			"Pop": 1234,
		},
		"World": map[interface{}]interface{}{
			"Rock": 345,
		},
	}

	if err := v.MergeConfigMap(update); err != nil {
		t.Fatal(err)
	}

	if rock := v.GetInt("world.rock"); rock != 345 {
		t.Fatal("Got rock:", rock)
	}

	assert(1234)

}

func TestUnmarshalingWithAliases(t *testing.T) {
	v := New()
	v.SetDefault("ID", 1)
	v.Set("name", "Steve")
	v.Set("lastname", "Owen")

	v.RegisterAlias("UserID", "ID")
	v.RegisterAlias("Firstname", "name")
	v.RegisterAlias("Surname", "lastname")

	type config struct {
		ID        int
		FirstName string
		Surname   string
	}

	var C config
	err := v.Unmarshal(&C)
	if err != nil {
		t.Fatalf("unable to decode into struct, %v", err)
	}

	assert.Equal(t, &config{ID: 1, FirstName: "Steve", Surname: "Owen"}, &C)
}

func TestSetConfigNameClearsFileCache(t *testing.T) {
	SetConfigFile("/tmp/config.yaml")
	SetConfigName("default")
	f, err := v.getConfigFile()
	if err == nil {
		t.Fatalf("config file cache should have been cleared")
	}
	assert.Empty(t, f)
}

func TestShadowedNestedValue(t *testing.T) {

	config := `name: steve
clothing:
  jacket: leather
  trousers: denim
  pants:
    size: large
`
	initConfig("yaml", config)

	assert.Equal(t, "steve", GetString("name"))

	polyester := "polyester"
	SetDefault("clothing.shirt", polyester)
	SetDefault("clothing.jacket.price", 100)

	assert.Equal(t, "leather", GetString("clothing.jacket"))
	assert.Nil(t, Get("clothing.jacket.price"))
	assert.Equal(t, polyester, GetString("clothing.shirt"))

	clothingSettings := AllSettings()["clothing"].(map[string]interface{})
	assert.Equal(t, "leather", clothingSettings["jacket"])
	assert.Equal(t, polyester, clothingSettings["shirt"])
}

func TestDotParameter(t *testing.T) {
	initYAML()

	// shoud take precedence over batters defined in jsonExample
	r := bytes.NewReader([]byte("clothing.pants:\n  size: small"))
	_ = unmarshalReader(r, v.config)

	actual := Get("clothing.pants.size")
	expected := "small"
	assert.Equal(t, expected, actual)
}

func TestCaseInsensitive(t *testing.T) {
	for _, config := range []struct {
		typ     string
		content string
	}{
		{"yaml", `
aBcD: 1
eF:
  gH: 2
  iJk: 3
  Lm:
    nO: 4
    P:
      Q: 5
      R: 6
`},
	} {
		doTestCaseInsensitive(t, config.typ, config.content)
	}
}

func TestCaseInsensitiveSet(t *testing.T) {
	Reset()
	m1 := map[string]interface{}{
		"Foo": 32,
		"Bar": map[interface{}]interface {
		}{
			"ABc": "A",
			"cDE": "B"},
	}

	m2 := map[string]interface{}{
		"Foo": 52,
		"Bar": map[interface{}]interface {
		}{
			"bCd": "A",
			"eFG": "B"},
	}

	Set("Given1", m1)
	Set("Number1", 42)

	SetDefault("Given2", m2)
	SetDefault("Number2", 52)

	// Verify SetDefault
	if v := Get("number2"); v != 52 {
		t.Fatalf("Expected 52 got %q", v)
	}

	if v := Get("given2.foo"); v != 52 {
		t.Fatalf("Expected 52 got %q", v)
	}

	if v := Get("given2.bar.bcd"); v != "A" {
		t.Fatalf("Expected A got %q", v)
	}

	if _, ok := m2["Foo"]; !ok {
		t.Fatal("Input map changed")
	}

	// Verify Set
	if v := Get("number1"); v != 42 {
		t.Fatalf("Expected 42 got %q", v)
	}

	if v := Get("given1.foo"); v != 32 {
		t.Fatalf("Expected 32 got %q", v)
	}

	if v := Get("given1.bar.abc"); v != "A" {
		t.Fatalf("Expected A got %q", v)
	}

	if _, ok := m1["Foo"]; !ok {
		t.Fatal("Input map changed")
	}
}

func TestParseNested(t *testing.T) {
	type duration struct {
		Delay time.Duration
	}

	type item struct {
		Name   string
		Delay  time.Duration
		Nested duration
	}

	config := `
parent:
  delay: 100ms
  nested:
    delay: 200ms
`
	initConfig("yaml", config)

	var items []item
	err := v.UnmarshalKey("parent", &items)
	if err != nil {
		t.Fatalf("unable to decode into struct, %v", err)
	}

	assert.Equal(t, 1, len(items))
	assert.Equal(t, 100*time.Millisecond, items[0].Delay)
	assert.Equal(t, 200*time.Millisecond, items[0].Nested.Delay)
}

func doTestCaseInsensitive(t *testing.T, typ, config string) {
	initConfig(typ, config)
	Set("RfD", true)
	assert.Equal(t, true, Get("rfd"))
	assert.Equal(t, true, Get("rFD"))
	assert.Equal(t, 1, cast.ToInt(Get("abcd")))
	assert.Equal(t, 1, cast.ToInt(Get("Abcd")))
	assert.Equal(t, 2, cast.ToInt(Get("ef.gh")))
	assert.Equal(t, 3, cast.ToInt(Get("ef.ijk")))
	assert.Equal(t, 4, cast.ToInt(Get("ef.lm.no")))
	assert.Equal(t, 5, cast.ToInt(Get("ef.lm.p.q")))
}

func BenchmarkGetBool(b *testing.B) {
	key := "BenchmarkGetBool"
	v = New()
	v.Set(key, true)

	for i := 0; i < b.N; i++ {
		if !v.GetBool(key) {
			b.Fatal("GetBool returned false")
		}
	}
}

func BenchmarkGet(b *testing.B) {
	key := "BenchmarkGet"
	v = New()
	v.Set(key, true)

	for i := 0; i < b.N; i++ {
		if !v.Get(key).(bool) {
			b.Fatal("Get returned false")
		}
	}
}

// BenchmarkGetBoolFromMap is the "perfect result" for the above.
func BenchmarkGetBoolFromMap(b *testing.B) {
	m := make(map[string]bool)
	key := "BenchmarkGetBool"
	m[key] = true

	for i := 0; i < b.N; i++ {
		if !m[key] {
			b.Fatal("Map value was false")
		}
	}
}
