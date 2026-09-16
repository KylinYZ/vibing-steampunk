package adt

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// dataElementDoc is the ADT document served by
// /sap/bc/adt/ddic/dataelements/{name}. XML namespaces identify the vocabulary,
// but their prefixes are chosen by the server, so the wire representation uses
// local names only.
type dataElementDoc struct {
	XMLName     xml.Name
	Name        string                 `xml:"name,attr"`
	// ObjectType is the root's workbench type attribute ("DTEL/DE"). The
	// packageRef child also carries an adtcore:type ("DEVC/K"); structured
	// root-attribute mapping cannot confuse the two, which is exactly why
	// ObjectType is read here rather than matched loosely.
	ObjectType  string                 `xml:"type,attr"`
	Description string                 `xml:"description,attr"`
	DataElement *dataElementProperties `xml:"dataElement"`
}

type dataElementProperties struct {
	TypeKind         string `xml:"typeKind"`
	TypeName         string `xml:"typeName"`
	DataType         string `xml:"dataType"`
	DataTypeLength   string `xml:"dataTypeLength"`
	DataTypeDecimals string `xml:"dataTypeDecimals"`
	Short            string `xml:"shortFieldLabel"`
	Medium           string `xml:"mediumFieldLabel"`
	Long             string `xml:"longFieldLabel"`
	Heading          string `xml:"headingFieldLabel"`
}

func parseDataElementDoc(body []byte) (*dataElementDoc, error) {
	var doc dataElementDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parsing data element document: %w", err)
	}
	if doc.XMLName.Local != "wbobj" {
		return nil, fmt.Errorf("not a data element document: root element is %q", doc.XMLName.Local)
	}
	if strings.TrimSpace(doc.Name) == "" {
		return nil, fmt.Errorf("data element document is missing name")
	}
	if doc.DataElement == nil {
		return nil, fmt.Errorf("data element document is missing dataElement")
	}
	return &doc, nil
}

func parseDataElementNumber(field, value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("data element document is missing %s", field)
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("data element document has invalid %s %q", field, value)
	}
	return n, nil
}
