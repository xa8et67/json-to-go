package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"json-to-go/jsonparser"
	"strconv"
	"strings"
	"unicode"
)

// 大类型 group
const (
	GroupV    = "Value"
	GroupV1   = "[]Value"
	GroupV2   = "[][]Value"
	GroupO    = "Object"
	GroupO1   = "[]Object"
	GroupO2   = "[][]Object"
	GroupNil1 = "[]"   // 临时类型
	GroupNil2 = "[][]" // 临时类型
)

// 小类型 type
const (
	TypeString  = "string"
	TypeBool    = "bool"
	TypeFloat64 = "float64"
	TypeInt     = "int"
	TypeInt64   = "int64"
	TypeAny     = "interface{}"
	TypeNil     = "nil" // 临时类型，属性为null的，数组为空的，都先用这个表示。最后再进行属性合并的时候会用到
)

const (
	Comment0 = iota
	Comment1
	Comment2
)

const (
	DefaultName = "AutoGenerated"
	DefaultTag  = "json"
	MaxInt32    = 1<<31 - 1
	MinInt32    = -1 << 31
)

// https://github.com/golang/lint/blob/master/lint.go
var commonInitialisms = map[string]struct{}{
	"ACL":   {},
	"API":   {},
	"ASCII": {},
	"CPU":   {},
	"CSS":   {},
	"DNS":   {},
	"EOF":   {},
	"GUID":  {},
	"HTML":  {},
	"HTTP":  {},
	"HTTPS": {},
	"ID":    {},
	"IP":    {},
	"JSON":  {},
	"LHS":   {},
	"QPS":   {},
	"RAM":   {},
	"RHS":   {},
	"RPC":   {},
	"SLA":   {},
	"SMTP":  {},
	"SQL":   {},
	"SSH":   {},
	"TCP":   {},
	"TLS":   {},
	"TTL":   {},
	"UDP":   {},
	"UI":    {},
	"UID":   {},
	"UUID":  {},
	"URI":   {},
	"URL":   {},
	"UTF8":  {},
	"VM":    {},
	"XML":   {},
	"XMPP":  {},
	"XSRF":  {},
	"XSS":   {},
}

type Config struct {
	// tags
	Tags []string
	// 0忽略注释，1生成单行注释 2生成行尾注释
	Comment int
	// 是否使用指针
	PointerFlag bool
	// 是否嵌套结构
	NestFlag bool
	// 控制是否生成访问函数
	AccessorFlag bool
	// 新增结构体类型选项
	StructType string
}

type Node struct {
	// 字段名称
	k string
	// 字段类型
	t string
	// 字段分组，所属大类型
	g string
	// 注释
	c string
	// 嵌套结构
	children *[]*Node
	// 用来merge的
	childrenMerge *[][]*Node
	// childrenMerge 下标使用
	cache map[string]int
	// 用于存储格式化后的结构体名称和字段名。
	formattedName string
	formattedKey  string
}

// Generate json字符串转对象，在前端进行了json5格式验证和格式化
func Generate(jsonStr string, config *Config) (string, error) {
	setJsonTag(config)
	// 添加类型判断
	if config.StructType == "map" {
		return generateMap(jsonStr, config)
	}
	// 解析JSON
	parent := NewNode(DefaultName, "", GroupO, "")
	var err error
	if jsonStr[0:1] == "[" {
		err = jsonparser.ArrayEach([]byte(jsonStr), func(value []byte, dataType jsonparser.ValueType, offset int, comment []byte) (bool, error) {
			if dataType == jsonparser.Object {
				err = recursionNode(parent, value, config)
				if err != nil {
					return false, err
				}
			}
			return true, nil
		})
	} else {
		err = recursionNode(parent, []byte(jsonStr), config)
	}
	if err != nil {
		fmt.Println(err)
		return err.Error(), err
	}
	// 合并数组内的对象和属性
	mergeArrayNode(parent)
	var buff bytes.Buffer
	all := make([]*Node, 0)
	if config.NestFlag {
		// 嵌套结构体
		buff.WriteString(fmt.Sprintf("type %s ", parent.k))
		nestKey := recursionWrite(parent, config)
		buff.WriteString(nestKey)
	} else {
		recursionAdd(&all, parent)
		// 格式化前name；格式化后name
		nameMap := make(map[string]string)
		// 转换后的name，如果重名了，后面加数字表示
		nameCount := make(map[string]int)
		for i, a := range all {
			// 设置格式化后的结构体名称
			formattedName := formatKey(nameMap, nameCount, a.k)
			a.formattedName = formattedName

			buff.WriteString(fmt.Sprintf("type %s struct {\n", formatKey(nameMap, nameCount, a.k)))
			for _, node := range *a.children {
				if node.c != "" && config.Comment == Comment1 {
					buff.WriteString(node.c + "\n")
				}
				// 设置格式化后的字段名称
				key := formatKey(nameMap, nameCount, node.k)
				node.formattedKey = key
				if node.c != "" && config.Comment == Comment2 {
					buff.WriteString(fmt.Sprintf("%s %s %s %s\n", key, formatType(key, node.t, node.g, config.PointerFlag), formatTag(node.k, config.Tags), node.c))
				} else {
					buff.WriteString(fmt.Sprintf("%s %s %s\n", key, formatType(key, node.t, node.g, config.PointerFlag), formatTag(node.k, config.Tags)))
				}
			}
			if i == len(all)-1 {
				buff.WriteString("}")
			} else {
				buff.WriteString("}\n\n")
			}
		}
	}
	if config.AccessorFlag {
		if config.NestFlag {
			// 嵌套模式下只生成最外层结构体的访问函数
			generateAccessor(&buff, parent)
		} else {
			// 非嵌套模式下生成所有结构体的访问函数
			for _, a := range all {
				generateAccessor(&buff, a)
			}
		}
	}
	source, err := format.Source(buff.Bytes())
	if err != nil {
		fmt.Println(err)
		return err.Error(), err
	}
	return string(source), nil
}

func generateAccessor(buff *bytes.Buffer, node *Node) {
	if node.formattedName == "" {
		return
	}
	buff.WriteString(fmt.Sprintf("\nfunc (n *%s) GetField(fieldName string) interface{} {\n", node.formattedName))
	buff.WriteString("    switch fieldName {\n")
	for _, child := range *node.children {
		if child.formattedKey == "" {
			continue
		}
		buff.WriteString(fmt.Sprintf("    case %q:\n        return n.%s\n", child.formattedKey, child.formattedKey))
	}
	buff.WriteString("    default:\n        return nil\n    }\n}\n")
}

// 新增Map生成函数
func generateMap(jsonStr string, config *Config) (string, error) {
	var result interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return "", err
	}

	var buff bytes.Buffer
	buff.WriteString("var generatedMap = ")
	generateMapValue(&buff, result, 0)
	source, _ := format.Source(buff.Bytes())
	return string(source), nil
}

// 递归生成Map结构
func generateMapValue(buff *bytes.Buffer, value interface{}, indent int) {
	switch v := value.(type) {
	case map[string]interface{}:
		buff.WriteString("map[string]interface{}{\n")
		for key, val := range v {
			buff.WriteString(strings.Repeat("    ", indent+1))
			fmt.Fprintf(buff, "%q: ", key)
			generateMapValue(buff, val, indent+1)
			buff.WriteString(",\n")
		}
		buff.WriteString(strings.Repeat("    ", indent) + "}")
	case []interface{}:
		buff.WriteString("[]interface{}{\n")
		for _, item := range v {
			buff.WriteString(strings.Repeat("    ", indent+1))
			generateMapValue(buff, item, indent+1)
			buff.WriteString(",\n")
		}
		buff.WriteString(strings.Repeat("    ", indent) + "}")
	case float64:
		// 判断是否为整数
		if v == float64(int64(v)) {
			buff.WriteString(fmt.Sprintf("%d", int64(v))) // 强制转换为整数
		} else {
			buff.WriteString(fmt.Sprintf("%v", v)) // 保持浮点数
		}
	default:
		fmt.Fprintf(buff, "%#v", v)
	}
}

func NewNode(k, t, g, c string) *Node {
	node := &Node{
		k: k,
		t: t,
		g: g,
		c: c,
	}
	node.children = &[]*Node{}
	node.childrenMerge = &[][]*Node{}
	node.cache = make(map[string]int)
	return node
}

func mergeArrayNode(parent *Node) {
	for _, node := range *parent.childrenMerge {
		addChildren(parent, walkNode(node))
	}
}

// nodes是一个属性
func walkNode(nodes []*Node) *Node {
	parent := mergeNode(nodes)
	for _, node := range *parent.childrenMerge {
		addChildren(parent, walkNode(node))
	}
	return parent
}

func mergeNode(nodes []*Node) *Node {
	n := NewNode(nodes[0].k, "", "", "")
	group, t := mergeGroupAndType(nodes)
	n.g = group
	n.t = t
	n.c = mergeComment(nodes)
	for _, node := range nodes {
		for _, n1 := range *node.childrenMerge {
			for _, n2 := range n1 {
				addChildrenMerge(n, n2)
			}
		}
	}
	return n
}

func setJsonTag(config *Config) {
	flag := false
	for _, tag := range config.Tags {
		if tag == DefaultTag {
			flag = true
			break
		}
	}
	if !flag {
		if config.Tags == nil {
			config.Tags = make([]string, 0)
		}
		config.Tags = append([]string{DefaultTag}, config.Tags...)
	}
}

func recursionAdd(all *[]*Node, node *Node) {
	// 支持没有属性的struct
	if node.g == GroupO || node.g == GroupO1 || node.g == GroupO2 {
		*all = append(*all, node)
	}
	for _, n := range *node.children {
		recursionAdd(all, n)
	}
}

func recursionWrite(parent *Node, config *Config) string {
	// 格式化前name；格式化后name
	nameMap := make(map[string]string)
	// 转换后的name，如果重名了，后面加数字表示
	nameCount := make(map[string]int)
	var res bytes.Buffer
	res.WriteString("struct {\n")
	for _, node := range *parent.children {
		if node.c != "" && config.Comment == Comment1 {
			res.WriteString(node.c + "\n")
		}
		key := formatKey(nameMap, nameCount, node.k)
		nestKey := key
		if len(*node.children) > 0 {
			nestKey = recursionWrite(node, config)
		}
		if node.c != "" && config.Comment == Comment2 {
			res.WriteString(fmt.Sprintf("%s %s %s %s\n", key, formatType(nestKey, node.t, node.g, config.PointerFlag), formatTag(node.k, config.Tags), node.c))
		} else {
			res.WriteString(fmt.Sprintf("%s %s %s\n", key, formatType(nestKey, node.t, node.g, config.PointerFlag), formatTag(node.k, config.Tags)))
		}
	}
	res.WriteString("}")
	return res.String()
}

func mergeComment(nodes []*Node) string {
	comment := ""
	for _, p := range nodes {
		// 注释可能是重复的，取第一个
		if comment == "" && p.c != "" {
			comment = p.c
		}
	}
	return comment
}

// 返回属性的类型，需要考虑大类型和小类型
func mergeGroupAndType(array []*Node) (group string, t string) {
	var groups []string
	var types []string
	for _, p := range array {
		groups = append(groups, p.g)
		types = append(types, p.t)
	}
	flag, group := mergeFiledGroup(groups)
	if flag {
		// type类型确定
		return group, TypeAny
	}
	if isObject(group) {
		// 对象类型，不需要t
		return group, TypeAny
	}
	// 判断type
	t = mergeFiledType(types, false)
	return group, t
}

// 获取大类型group，判断小类型是否是any
func mergeFiledGroup(array []string) (bool, string) {
	VFlag := false
	V1Flag := false
	V2Flag := false
	OFlag := false
	O1Flag := false
	O2Flag := false
	Nil1Flag := false
	Nil2Flag := false
	for _, g := range array {
		switch g {
		case GroupV:
			VFlag = true
		case GroupV1:
			V1Flag = true
		case GroupV2:
			V2Flag = true
		case GroupO:
			OFlag = true
		case GroupO1:
			O1Flag = true
		case GroupO2:
			O2Flag = true
		case GroupNil1:
			Nil1Flag = true
		case GroupNil2:
			Nil2Flag = true
		}
	}
	count := 0
	if VFlag {
		count++
	}
	if V1Flag {
		count++
	}
	if V2Flag {
		count++
	}
	if OFlag {
		count++
	}
	if O1Flag {
		count++
	}
	if O2Flag {
		count++
	}
	// 类型是any的情况 返回any
	if count > 1 || Nil1Flag && Nil2Flag {
		return true, GroupV
	}
	if Nil1Flag && (VFlag || V2Flag || OFlag || O2Flag) {
		return true, GroupV
	}
	if Nil2Flag && (VFlag || V1Flag || OFlag || O1Flag) {
		return true, GroupV
	}
	if count == 0 {
		if Nil1Flag {
			// 返回 []any
			return true, GroupV1
		} else {
			// 返回 [][]any
			return true, GroupV2
		}
	}
	// 类型不是any
	if VFlag {
		return false, GroupV
	} else if V1Flag {
		// 不需要考虑Nil1Flag
		return false, GroupV1
	} else if V2Flag {
		return false, GroupV2
	} else if OFlag {
		return false, GroupO
	} else if O1Flag {
		return false, GroupO1
	} else {
		return false, GroupO2
	}
}

// 格式化对象名，属性名，并且通过cache来解决全局重名问题
// 对象名不能重复；属性名要考虑多个对象之间的一致性，所以要保持全局统一的映射关系
func formatKey(nameMap map[string]string, nameCount map[string]int, key string) string {
	if e, ok := nameMap[key]; ok {
		return e
	}
	result := ""
	// 将驼峰式命名转换为下划线分割
	newKey := convertToUnderline(key)
	// 按下划线分割，每个片段的首字母大写
	split := strings.Split(newKey, "_")
	for i, str := range split {
		span := ""
		for j, v := range str {
			s := ""
			if isLetter(v) {
				s = string(v)
			} else if isDigit(v) {
				s = string(v)
			} else if unicode.Is(unicode.Han, v) {
				// 处理中文字符
				s = convertToPinyin(string(v))
			}
			if s == "" {
				continue
			}
			if j == 0 {
				s = strings.ToUpper(s)
			}
			if i == 0 && span == "" {
				// 首字母如果是数字，就转换
				s = numToLetter(s)
			}
			span += s
		}
		span = convertInitialisms(span)
		result += span
	}
	result = convertInitialisms(result)
	// 判断是否重名
	if count, ok := nameCount[result]; ok {
		// 重名了，末尾加数字
		result = result + strconv.Itoa(count+1)
		nameCount[result] = count + 1
	} else {
		nameCount[result] = 0
	}
	nameMap[key] = result
	return result
}

func convertToUnderline(key string) string {
	var buffer bytes.Buffer
	runes := []rune(key)
	length := len(runes)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 && i+1 < length && unicode.IsLower(runes[i+1]) {
				buffer.WriteRune('_')
				buffer.WriteRune(unicode.ToLower(r))
			} else {
				buffer.WriteRune(r)
			}
		} else {
			buffer.WriteRune(r)
		}
	}
	return buffer.String()
}

func isLetter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// 中文字符转拼音首字母，返回小写
func convertToPinyin(s string) string {
	return GetPinYin(s)
}

// 格式化完整的类型
func formatType(key string, t string, group string, pointerFlag bool) string {
	result := t
	pointer := ""
	if pointerFlag {
		pointer = "*"
	}
	if group == GroupO {
		result = pointer + key
	} else if group == GroupO1 {
		result = "[]" + pointer + key
	} else if group == GroupO2 {
		result = "[][]" + pointer + key
	} else if group == GroupV1 {
		result = "[]" + t
	} else if group == GroupV2 {
		result = "[][]" + t
	}
	return result
}

// 格式化tag
func formatTag(key string, tag []string) string {
	result := "`"
	var array []string
	for _, t := range tag {
		s := fmt.Sprintf("%s:%q", t, key)
		array = append(array, s)
	}
	result += strings.Join(array, " ")
	result += "`"
	return result
}

func recursionNode(parent *Node, data []byte, config *Config) error {
	var err error
	var group, t, c string
	var arrayObj [][]byte
	err = jsonparser.ObjectEach(data, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int, comment []byte) (flag bool, err error) {
		group, err = getGroup(value, dataType)
		if err != nil {
			return false, err
		}
		switch group {
		case GroupV:
			addChildrenMerge(parent, NewNode(string(key), getJSONType(value, dataType), group, string(comment)))
		case GroupV1:
			t, c, err = getJSONArrayType(value, 1)
			if err != nil {
				return false, err
			}
			// 优先使用数组的注释，不存在时，在使用从属性里提取出来的注释
			if len(comment) > 0 {
				c = string(comment)
			}
			addChildrenMerge(parent, NewNode(string(key), t, group, c))
		case GroupV2:
			t, c, err = getJSONArrayType(value, 2)
			if err != nil {
				return false, err
			}
			if len(comment) > 0 {
				c = string(comment)
			}
			addChildrenMerge(parent, NewNode(string(key), t, group, c))
		case GroupO:
			node := NewNode(string(key), string(key), group, string(comment))
			addChildrenMerge(parent, node)
			err = recursionNode(node, value, config)
			if err != nil {
				return false, err
			}
		case GroupO1:
			arrayObj, c, err = getArrayObj(value, 1)
			if err != nil {
				return false, err
			}
			if len(comment) > 0 {
				c = string(comment)
			}

			node := NewNode(string(key), string(key), group, c)
			addChildrenMerge(parent, node)

			for _, obj := range arrayObj {
				err = recursionNode(node, obj, config)
				if err != nil {
					return false, err
				}
			}
		case GroupO2:
			arrayObj, c, err = getArrayObj(value, 2)
			if err != nil {
				return false, err
			}
			if len(comment) > 0 {
				c = string(comment)
			}

			node := NewNode(string(key), string(key), group, c)
			addChildrenMerge(parent, node)

			for _, obj := range arrayObj {
				err = recursionNode(node, obj, config)
				if err != nil {
					return false, err
				}
			}
		case GroupNil1:
			addChildrenMerge(parent, NewNode(string(key), TypeNil, group, ""))
		case GroupNil2:
			addChildrenMerge(parent, NewNode(string(key), TypeNil, group, ""))
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	return nil
}

func getGroup(value []byte, dataType jsonparser.ValueType) (string, error) {
	group := ""
	var err error
	if dataType == jsonparser.Array {
		count1 := 0
		// 二维数组的第一个元素如果是空，继续判断
		nextFlag := true
		err = jsonparser.ArrayEach(value, func(value2 []byte, dataType2 jsonparser.ValueType, offset2 int, comment []byte) (flag bool, err error) {
			count1++
			if nextFlag {
				if dataType2 == jsonparser.Object {
					group = GroupO1
					nextFlag = false
				} else if dataType2 == jsonparser.Array {
					count2 := 0
					err = jsonparser.ArrayEach(value2, func(value3 []byte, dataType3 jsonparser.ValueType, offset3 int, comment []byte) (flag bool, err error) {
						count2++
						nextFlag = false
						if dataType3 == jsonparser.Object {
							group = GroupO2
						} else {
							group = GroupV2
						}
						return false, nil
					})
					if err != nil {
						return false, err
					}
					if count1 == 1 && count2 == 0 {
						group = GroupNil2
					}
				} else {
					group = GroupV1
					nextFlag = false
				}
			}
			return nextFlag, nil
		})
		if err != nil {
			return "", err
		}
		if count1 == 0 {
			group = GroupNil1
		}
	} else if dataType == jsonparser.Object {
		group = GroupO
	} else {
		group = GroupV
	}
	return group, nil
}

func addChildren(parent *Node, node *Node) {
	*parent.children = append(*parent.children, node)
}

func addChildrenMerge(parent *Node, node *Node) {
	if index, ok := parent.cache[node.k]; ok {
		(*parent.childrenMerge)[index] = append((*parent.childrenMerge)[index], node)
	} else {
		length := len(*parent.childrenMerge)
		*parent.childrenMerge = append(*parent.childrenMerge, []*Node{node})
		parent.cache[node.k] = length
	}
}

// 获取数组内所有的对象
func getArrayObj(data []byte, count int) (result [][]byte, c string, err error) {
	err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, comment []byte) (flag bool, err error) {
		if count == 1 {
			result = append(result, value)
		} else {
			err = jsonparser.ArrayEach(value, func(value2 []byte, dataType jsonparser.ValueType, offset int, comment []byte) (flag bool, err error) {
				result = append(result, value2)
				return true, nil
			})
			if err != nil {
				return false, err
			}
		}
		// 注释只提取外层的
		if c == "" && string(comment) != "" {
			c = string(comment)
		}
		return true, nil
	})
	if err != nil {
		return nil, c, err
	}
	return result, c, nil
}

// 合并多个type类型
func mergeFiledType(array []string, flag bool) string {
	stringFlag := false
	boolFlag := false
	float64Flag := false
	intFlag := false
	int64Flag := false
	anyFlag := false
	nilFlag := false
	for _, t := range array {
		switch t {
		case TypeString:
			stringFlag = true
		case TypeBool:
			boolFlag = true
		case TypeFloat64:
			float64Flag = true
		case TypeInt:
			intFlag = true
		case TypeInt64:
			int64Flag = true
		case TypeAny:
			anyFlag = true
		case TypeNil:
			nilFlag = true
		}
	}
	if anyFlag {
		return TypeAny
	}
	count := 0
	// 将这三种类型，统一合并为数字类型
	if float64Flag || intFlag || int64Flag {
		count++
	}
	if stringFlag {
		count++
	}
	if boolFlag {
		count++
	}
	if count > 1 {
		// 代表出现了不同的类型
		return TypeAny
	}
	if stringFlag {
		return TypeString
	} else if boolFlag {
		return TypeBool
	} else if float64Flag {
		// 优先级  float64>int64>int
		return TypeFloat64
	} else if int64Flag {
		return TypeInt64
	} else if intFlag {
		return TypeInt
	} else if nilFlag {
		// 空类型优先级最低，这个地方返回空类型，为了后续类型合并使用
		if flag {
			return TypeNil
		}
		return TypeAny
	}
	return TypeAny
}

// 合并数组内所有属性的类型
func getJSONArrayType(data []byte, count int) (result string, c string, err error) {
	// 通过数组来推断类型
	var array []string
	err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, comment []byte) (flag bool, err error) {
		if count == 1 {
			jsonType := getJSONType(value, dataType)
			array = append(array, jsonType)
		} else {
			err = jsonparser.ArrayEach(value, func(value2 []byte, dataType jsonparser.ValueType, offset int, comment []byte) (flag bool, err error) {
				jsonType := getJSONType(value2, dataType)
				array = append(array, jsonType)
				return true, nil
			})
			if err != nil {
				return false, err
			}
		}
		// 注释只提取外层的
		if c == "" && string(comment) != "" {
			c = string(comment)
		}
		return true, nil
	})
	if err != nil {
		return "", c, err
	}
	return mergeFiledType(array, true), c, nil
}

// 获取json属性的类型
func getJSONType(value []byte, t jsonparser.ValueType) string {
	str := TypeAny
	if t == jsonparser.Number {
		str = TypeFloat64
		v := string(value)
		if !strings.Contains(v, ".") {
			// 是整数
			i, _ := strconv.Atoi(v)
			if i >= MinInt32 && i <= MaxInt32 {
				str = TypeInt
			} else {
				str = TypeInt64
			}
		}
	} else if t == jsonparser.Boolean {
		str = TypeBool
	} else if t == jsonparser.String {
		str = TypeString
	} else if t == jsonparser.Null {
		str = TypeNil
	}
	return str
}

func numToLetter(s string) string {
	switch s {
	case "0":
		return "Zero"
	case "1":
		return "One"
	case "2":
		return "Two"
	case "3":
		return "Three"
	case "4":
		return "Four"
	case "5":
		return "Five"
	case "6":
		return "Six"
	case "7":
		return "Seven"
	case "8":
		return "Eight"
	case "9":
		return "Nine"
	}
	return s
}

func convertInitialisms(s string) string {
	upper := strings.ToUpper(s)
	if _, ok := commonInitialisms[upper]; ok {
		return upper
	}
	return s
}

func isObject(group string) bool {
	if group == GroupO || group == GroupO1 || group == GroupO2 {
		return true
	}
	return false
}
