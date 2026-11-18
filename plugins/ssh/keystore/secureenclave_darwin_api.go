//go:build darwin

package keystore

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Core Foundation and Security framework constants used below.
const (
	securityPath       = "/System/Library/Frameworks/Security.framework/Security"
	coreFoundationPath = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"

	cfStringEncodingUTF8  = 0x08000100
	cfNumberSInt32Type    = 3
	accessPrivateKeyUsage = 1 << 30 // kSecAccessControlPrivateKeyUsage.
	errorBufferSize       = 1024
)

// secAPI holds the bound C functions and the CFTypeRef values of the constants. kSec... and kCF...
// constants are data symbols: Dlsym returns the address of the variable, which is dereferenced to
// get the CFTypeRef.
type secAPI struct {
	// Core Foundation functions.
	cfRelease          func(ref uintptr)
	cfDictionaryCreate func(alloc uintptr, keys, values unsafe.Pointer, n int, keyCB, valueCB uintptr) uintptr
	cfDictionaryGet    func(dict, key uintptr) uintptr
	cfDataCreate       func(alloc uintptr, bytes unsafe.Pointer, length int) uintptr
	cfDataGetLength    func(data uintptr) int
	cfDataGetBytePtr   func(data uintptr) uintptr
	cfNumberCreate     func(alloc uintptr, kind int, value unsafe.Pointer) uintptr
	cfStringCreate     func(alloc uintptr, cstr string, encoding uint32) uintptr
	cfStringGetCString func(str uintptr, buf unsafe.Pointer, size int, encoding uint32) bool
	cfErrorGetCode     func(err uintptr) int
	cfErrorGetDomain   func(err uintptr) uintptr
	cfErrorCopyDesc    func(err uintptr) uintptr

	// Security functions.
	accessControlCreate func(alloc, protection uintptr, flags uint, err unsafe.Pointer) uintptr
	keyCreateRandom     func(params uintptr, err unsafe.Pointer) uintptr
	keyCopyAttributes   func(key uintptr) uintptr
	keyCreateWithData   func(data, attrs uintptr, err unsafe.Pointer) uintptr
	keyCopyPublicKey    func(key uintptr) uintptr
	keyCopyExternalRep  func(key uintptr, err unsafe.Pointer) uintptr
	keyCreateSignature  func(key, algorithm, data uintptr, err unsafe.Pointer) uintptr

	// Callback structs (addresses, passed as is).
	dictKeyCallbacks   uintptr
	dictValueCallbacks uintptr

	// Constants (dereferenced CFTypeRefs).
	cfFalse                  uintptr
	attrKeyType              uintptr
	attrKeyTypeECSECPrime    uintptr
	attrKeySizeInBits        uintptr
	attrTokenID              uintptr
	attrTokenIDSecureEnclave uintptr
	privateKeyAttrs          uintptr
	attrIsPermanent          uintptr
	attrAccessControl        uintptr
	attrKeyClass             uintptr
	attrKeyClassPrivate      uintptr
	accessibleAfterFirst     uintptr
	accessibleWhenUnlocked   uintptr
	algECDSADigestSHA256     uintptr
	tokenOIDKey              uintptr // The CFString "toid" (kSecAttrTokenOID).
}

var (
	apiOnce sync.Once
	apiInst *secAPI
	apiErr  error
)

// loadAPI loads the frameworks once and binds every symbol the backend uses.
func loadAPI() (*secAPI, error) {
	apiOnce.Do(func() {
		apiInst, apiErr = bindAPI()
	})
	return apiInst, apiErr
}

type binder struct {
	lib uintptr
	err error
}

func (b *binder) fn(fptr any, name string) {
	if b.err != nil {
		return
	}
	addr, err := purego.Dlsym(b.lib, name)
	if err != nil {
		b.err = fmt.Errorf("resolve %s: %w", name, err)
		return
	}
	purego.RegisterFunc(fptr, addr)
}

// addr returns the address of a data symbol.
func (b *binder) addr(name string) uintptr {
	if b.err != nil {
		return 0
	}
	a, err := purego.Dlsym(b.lib, name)
	if err != nil {
		b.err = fmt.Errorf("resolve %s: %w", name, err)
	}
	return a
}

// value returns the CFTypeRef stored in a data symbol.
func (b *binder) value(name string) uintptr {
	a := b.addr(name)
	if a == 0 {
		return 0
	}
	return *(*uintptr)(addrToPointer(a))
}

// addrToPointer converts a raw address from Dlsym or a C call into an unsafe.Pointer without the
// uintptr-to-Pointer conversion that go vet rejects. The memory belongs to C, not the Go heap.
func addrToPointer(a uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&a))
}

func bindAPI() (*secAPI, error) {
	cf, err := purego.Dlopen(coreFoundationPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("%w: load CoreFoundation: %w", ErrUnsupportedPlatform, err)
	}
	sec, err := purego.Dlopen(securityPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("%w: load Security: %w", ErrUnsupportedPlatform, err)
	}
	a := &secAPI{}
	c := &binder{lib: cf}
	c.fn(&a.cfRelease, "CFRelease")
	c.fn(&a.cfDictionaryCreate, "CFDictionaryCreate")
	c.fn(&a.cfDictionaryGet, "CFDictionaryGetValue")
	c.fn(&a.cfDataCreate, "CFDataCreate")
	c.fn(&a.cfDataGetLength, "CFDataGetLength")
	c.fn(&a.cfDataGetBytePtr, "CFDataGetBytePtr")
	c.fn(&a.cfNumberCreate, "CFNumberCreate")
	c.fn(&a.cfStringCreate, "CFStringCreateWithCString")
	c.fn(&a.cfStringGetCString, "CFStringGetCString")
	c.fn(&a.cfErrorGetCode, "CFErrorGetCode")
	c.fn(&a.cfErrorGetDomain, "CFErrorGetDomain")
	c.fn(&a.cfErrorCopyDesc, "CFErrorCopyDescription")
	a.dictKeyCallbacks = c.addr("kCFTypeDictionaryKeyCallBacks")
	a.dictValueCallbacks = c.addr("kCFTypeDictionaryValueCallBacks")
	a.cfFalse = c.value("kCFBooleanFalse")
	if c.err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupportedPlatform, c.err)
	}

	s := &binder{lib: sec}
	s.fn(&a.accessControlCreate, "SecAccessControlCreateWithFlags")
	s.fn(&a.keyCreateRandom, "SecKeyCreateRandomKey")
	s.fn(&a.keyCopyAttributes, "SecKeyCopyAttributes")
	s.fn(&a.keyCreateWithData, "SecKeyCreateWithData")
	s.fn(&a.keyCopyPublicKey, "SecKeyCopyPublicKey")
	s.fn(&a.keyCopyExternalRep, "SecKeyCopyExternalRepresentation")
	s.fn(&a.keyCreateSignature, "SecKeyCreateSignature")
	a.attrKeyType = s.value("kSecAttrKeyType")
	a.attrKeyTypeECSECPrime = s.value("kSecAttrKeyTypeECSECPrimeRandom")
	a.attrKeySizeInBits = s.value("kSecAttrKeySizeInBits")
	a.attrTokenID = s.value("kSecAttrTokenID")
	a.attrTokenIDSecureEnclave = s.value("kSecAttrTokenIDSecureEnclave")
	a.privateKeyAttrs = s.value("kSecPrivateKeyAttrs")
	a.attrIsPermanent = s.value("kSecAttrIsPermanent")
	a.attrAccessControl = s.value("kSecAttrAccessControl")
	a.attrKeyClass = s.value("kSecAttrKeyClass")
	a.attrKeyClassPrivate = s.value("kSecAttrKeyClassPrivate")
	a.accessibleAfterFirst = s.value("kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly")
	a.accessibleWhenUnlocked = s.value("kSecAttrAccessibleWhenUnlockedThisDeviceOnly")
	a.algECDSADigestSHA256 = s.value("kSecKeyAlgorithmECDSASignatureDigestX962SHA256")
	if s.err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupportedPlatform, s.err)
	}
	// kSecAttrTokenOID is not exported as a constant on every SDK; its value is the string "toid".
	a.tokenOIDKey = a.cfStringCreate(0, "toid", cfStringEncodingUTF8)
	return a, nil
}

// releaser collects CF objects to release together.
type releaser struct {
	api  *secAPI
	refs []uintptr
}

func (r *releaser) add(ref uintptr) uintptr {
	if ref != 0 {
		r.refs = append(r.refs, ref)
	}
	return ref
}

func (r *releaser) release() {
	for i := len(r.refs) - 1; i >= 0; i-- {
		r.api.cfRelease(r.refs[i])
	}
	r.refs = nil
}

// dict builds an immutable CFDictionary from alternating key and value refs. The caller releases it.
func (a *secAPI) dict(pairs ...uintptr) uintptr {
	n := len(pairs) / 2
	keys := make([]uintptr, n)
	values := make([]uintptr, n)
	for i := 0; i < n; i++ {
		keys[i], values[i] = pairs[2*i], pairs[2*i+1]
	}
	d := a.cfDictionaryCreate(0, unsafe.Pointer(&keys[0]), unsafe.Pointer(&values[0]), n, a.dictKeyCallbacks, a.dictValueCallbacks)
	keepAlive(keys, values)
	return d
}

//go:noinline
func keepAlive(...any) {}

// data copies b into a new CFData. The caller releases it.
func (a *secAPI) data(b []byte) uintptr {
	if len(b) == 0 {
		return a.cfDataCreate(0, nil, 0)
	}
	d := a.cfDataCreate(0, unsafe.Pointer(&b[0]), len(b))
	keepAlive(b)
	return d
}

// number makes a CFNumber holding a 32-bit integer. The caller releases it.
func (a *secAPI) number(v int32) uintptr {
	n := v
	r := a.cfNumberCreate(0, cfNumberSInt32Type, unsafe.Pointer(&n))
	keepAlive(&n)
	return r
}

// bytesOf copies the contents of a CFData into Go memory.
func (a *secAPI) bytesOf(data uintptr) []byte {
	n := a.cfDataGetLength(data)
	if n == 0 {
		return nil
	}
	p := addrToPointer(a.cfDataGetBytePtr(data))
	return append([]byte(nil), unsafe.Slice((*byte)(p), n)...)
}

func (a *secAPI) goString(str uintptr) string {
	if str == 0 {
		return ""
	}
	buf := make([]byte, errorBufferSize)
	if !a.cfStringGetCString(str, unsafe.Pointer(&buf[0]), len(buf), cfStringEncodingUTF8) {
		return ""
	}
	for i, c := range buf {
		if c == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// toError converts a CFError into an *OSStatusError that keeps the code, and releases it.
func (a *secAPI) toError(op string, cfErr uintptr) error {
	if cfErr == 0 {
		return &OSStatusError{Op: op, Message: "unknown error"}
	}
	desc := a.cfErrorCopyDesc(cfErr)
	defer a.cfRelease(cfErr)
	if desc != 0 {
		defer a.cfRelease(desc)
	}
	// The domain is a get-rule reference owned by the error, so it is not released.
	return &OSStatusError{
		Op:      op,
		Code:    a.cfErrorGetCode(cfErr),
		Domain:  a.goString(a.cfErrorGetDomain(cfErr)),
		Message: a.goString(desc),
	}
}
