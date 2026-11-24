package saml2aws

import (
	"fmt"
	"strconv"
	"time"

	"github.com/beevik/etree"
)

const (
	assertionTag          = "Assertion"
	attributeStatementTag = "AttributeStatement"
	attributeTag          = "Attribute"
	attributeValueTag     = "AttributeValue"
	responseTag           = "Response"
)

// ErrMissingElement is the error type that indicates an element and/or attribute is
// missing. It provides a structured error that can be more appropriately acted
// upon.
type ErrMissingElement struct {
	Tag, Attribute string
}

// ErrMissingAssertion indicates that an appropriate assertion element could not
// be found in the SAML Response
var (
	ErrMissingAssertion = ErrMissingElement{Tag: assertionTag}
)

func (e ErrMissingElement) Error() string {
	if e.Attribute != "" {
		return fmt.Sprintf("missing %s attribute on %s element", e.Attribute, e.Tag)
	}
	return fmt.Sprintf("missing %s element", e.Tag)
}

// ExtractSessionDuration this will attempt to extract a session duration from the assertion
// see https://aws.amazon.com/SAML/Attributes/SessionDuration
func ExtractSessionDuration(data []byte) (int64, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return 0, err
	}

	assertionElement := doc.FindElement(".//Assertion")
	if assertionElement == nil {
		return 0, ErrMissingAssertion
	}

	// log.Printf("tag: %s", assertionElement.Tag)

	// Get the actual assertion attributes
	attributes, err := assertionAttributes(assertionElement)
	if err != nil {
		return 0, err
	}

	for _, attribute := range attributes {
		if attribute.SelectAttrValue("Name", "") != "https://aws.amazon.com/SAML/Attributes/SessionDuration" {
			continue
		}
		atributeValues := childElements(assertionElement, attribute, attributeValueTag)
		for _, attrValue := range atributeValues {
			return strconv.ParseInt(attrValue.Text(), 10, 64)
		}
	}

	return 0, nil
}

// ExtractDestinationURL will find the Destination URL to POST the SAML assertion to.
// This is necessary to support AWS instances with custom endpoints such as GovCloud and AWS China without requiring
// hardcoded endpoints on the saml2aws side.
func ExtractDestinationURL(data []byte) (string, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return "", err
	}

	rootElement := doc.Root()
	if rootElement == nil {
		return "", ErrMissingElement{Tag: responseTag}
	}

	destination := rootElement.SelectAttrValue("Destination", "none")
	if destination == "none" {
		// If Destination attribute is not found in Response (root) element,
		// get the Recipient attribute from the SubjectConfirmationData element.
		subjectConfirmationDataElement := doc.FindElement(".//SubjectConfirmationData")
		if subjectConfirmationDataElement == nil {
			return "", ErrMissingElement{Tag: responseTag}
		}

		destination = subjectConfirmationDataElement.SelectAttrValue("Recipient", "none")
		if destination == "none" {
			return "", ErrMissingElement{Tag: responseTag}
		}
	}

	return destination, nil
}

// ExtractMFATokenExpiryTime returns the duration of MFA token
// This is done by looking at the SubjectConfirmationData's NotOnOrAfter attribute
func ExtractMFATokenExpiryTime(data []byte) (time.Time, error) {
	var t time.Time

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return t, err
	}

	rootElement := doc.Root()
	if rootElement == nil {
		return t, ErrMissingElement{Tag: responseTag}
	}

	subjectConfirmationDataElement := doc.FindElement(".//SubjectConfirmationData")
	if subjectConfirmationDataElement == nil {
		return t, ErrMissingElement{Tag: responseTag}
	}

	// The field NotOnOrAfter holds the timestamp past which the token is invalid
	ValidUntilString := subjectConfirmationDataElement.SelectAttrValue("NotOnOrAfter", "")

	// return time.Parse("", ValidUntilString)
	return time.Parse(time.RFC3339, ValidUntilString)
}

// ExtractAwsRoles given an assertion document extract the aws roles
func ExtractAwsRoles(data []byte) ([]string, error) {
	awsroles := []string{}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return awsroles, err
	}

	// log.Printf("root tag: %s", doc.Root().Tag)

	assertionElement := doc.FindElement(".//Assertion")
	if assertionElement == nil {
		return nil, ErrMissingAssertion
	}

	// log.Printf("tag: %s", assertionElement.Tag)

	// Get the actual assertion attributes
	attributes, err := assertionAttributes(assertionElement)
	if err != nil {
		return nil, err
	}

	for _, attribute := range attributes {
		if attribute.SelectAttrValue("Name", "") != "https://aws.amazon.com/SAML/Attributes/Role" {
			continue
		}
		atributeValues := childElements(assertionElement, attribute, attributeValueTag)
		for _, attrValue := range atributeValues {
			awsroles = append(awsroles, attrValue.Text())
		}
	}

	return awsroles, nil
}

// childElements returns the direct children of parent with the given local tag that belong to the same
// namespace URI as the assertion element.
//
// Atmos fork: upstream built a "./<assertion-prefix>:<tag>" path, and etree compares that prefix literally, so
// an assertion using one prefix for Assertion and another prefix for the same namespace on its children was
// rejected. Matching by namespace URI accepts any prefix (and default namespaces) bound to that URI.
func childElements(assertion, parent *etree.Element, tag string) []*etree.Element {
	namespace := assertion.NamespaceURI()
	var matches []*etree.Element
	for _, child := range parent.ChildElements() {
		// An element whose prefix is not bound to any namespace is tolerated, as upstream matched by tag alone.
		if child.Tag == tag && (child.NamespaceURI() == namespace || child.NamespaceURI() == "") {
			matches = append(matches, child)
		}
	}
	return matches
}

// assertionAttributes returns the Attribute elements of every AttributeStatement of the assertion, in document order.
//
// Atmos fork: SAML 2.0 permits several AttributeStatement elements, and upstream read only the first, so a Role
// attribute in a later statement was never found. ErrMissingElement is returned when there is no statement at all.
func assertionAttributes(assertion *etree.Element) ([]*etree.Element, error) {
	statements := childElements(assertion, assertion, attributeStatementTag)
	if len(statements) == 0 {
		return nil, ErrMissingElement{Tag: attributeStatementTag}
	}

	var attributes []*etree.Element
	for _, statement := range statements {
		attributes = append(attributes, childElements(assertion, statement, attributeTag)...)
	}
	return attributes, nil
}
