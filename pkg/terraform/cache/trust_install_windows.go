//go:build windows

package cache

import (
	"bytes"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"unsafe"

	errUtils "github.com/cloudposse/atmos/errors"
	"golang.org/x/sys/windows"
)

const cryptENotFound windows.Errno = 0x80092004

func installWindowsTrust(certPath string) error {
	der, err := readCertificateDER(certPath)
	if err != nil {
		return err
	}

	certCtx, err := windows.CertCreateCertificateContext(windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, &der[0], uint32(len(der)))
	if err != nil {
		return fmt.Errorf("%w: creating Windows certificate context: %w", errUtils.ErrInvalidConfig, err)
	}
	defer func() {
		_ = windows.CertFreeCertificateContext(certCtx)
	}()

	store, err := openCurrentUserRootStore()
	if err != nil {
		return err
	}
	defer func() {
		_ = windows.CertCloseStore(store, 0)
	}()

	var added *windows.CertContext
	if err := windows.CertAddCertificateContextToStore(store, certCtx, windows.CERT_STORE_ADD_REPLACE_EXISTING, &added); err != nil {
		return fmt.Errorf("%w: adding certificate to Windows CurrentUser Root store: %w", errUtils.ErrInvalidConfig, err)
	}
	if added != nil {
		_ = windows.CertFreeCertificateContext(added)
	}
	return nil
}

func removeWindowsTrust(certPath string) error {
	store, err := openCurrentUserRootStore()
	if err != nil {
		return err
	}
	defer func() {
		_ = windows.CertCloseStore(store, 0)
	}()

	der, err := readCertificateDER(certPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(der) > 0 {
		return removeWindowsTrustByDER(store, der)
	}
	return removeWindowsTrustBySubject(store, certCommonName)
}

func removeWindowsTrustBySubject(store windows.Handle, commonName string) error {
	subject, err := windows.UTF16PtrFromString(commonName)
	if err != nil {
		return fmt.Errorf("%w: encoding certificate subject: %w", errUtils.ErrInvalidConfig, err)
	}

	for {
		certCtx, err := windows.CertFindCertificateInStore(
			store,
			windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING,
			0,
			windows.CERT_FIND_SUBJECT_STR,
			unsafe.Pointer(subject),
			nil,
		)
		if err != nil {
			if errors.Is(err, cryptENotFound) {
				return nil
			}
			return fmt.Errorf("%w: finding certificate in Windows CurrentUser Root store: %w", errUtils.ErrInvalidConfig, err)
		}
		if certCtx == nil {
			return nil
		}
		if err := windows.CertDeleteCertificateFromStore(certCtx); err != nil {
			return fmt.Errorf("%w: removing certificate from Windows CurrentUser Root store: %w", errUtils.ErrInvalidConfig, err)
		}
	}
}

func removeWindowsTrustByDER(store windows.Handle, der []byte) error {
	targetHash := sha1.Sum(der)

	var prev *windows.CertContext
	for {
		certCtx, err := windows.CertEnumCertificatesInStore(store, prev)
		prev = certCtx
		if certCtx == nil {
			if err != nil && !errors.Is(err, cryptENotFound) {
				return fmt.Errorf("%w: enumerating Windows CurrentUser Root store: %w", errUtils.ErrInvalidConfig, err)
			}
			return nil
		}

		raw := unsafe.Slice(certCtx.EncodedCert, certCtx.Length)
		certHash := sha1.Sum(raw)
		if !bytes.Equal(certHash[:], targetHash[:]) {
			continue
		}

		dup := windows.CertDuplicateCertificateContext(certCtx)
		if dup == nil {
			continue
		}
		if err := windows.CertDeleteCertificateFromStore(dup); err != nil {
			return fmt.Errorf("%w: removing certificate from Windows CurrentUser Root store: %w", errUtils.ErrInvalidConfig, err)
		}
	}
}

func readCertificateDER(certPath string) ([]byte, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("%w: reading certificate: %w", errUtils.ErrInvalidConfig, err)
	}
	if block, _ := pem.Decode(data); block != nil {
		data = block.Bytes
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: certificate is empty: %s", errUtils.ErrInvalidConfig, certPath)
	}
	if _, err := x509.ParseCertificate(data); err != nil {
		return nil, fmt.Errorf("%w: parsing certificate: %w", errUtils.ErrInvalidConfig, err)
	}
	return data, nil
}

func openCurrentUserRootStore() (windows.Handle, error) {
	storeName, err := windows.UTF16PtrFromString("Root")
	if err != nil {
		return 0, fmt.Errorf("%w: encoding Windows certificate store name: %w", errUtils.ErrInvalidConfig, err)
	}
	store, err := windows.CertOpenStore(
		windows.CERT_STORE_PROV_SYSTEM,
		0,
		0,
		windows.CERT_SYSTEM_STORE_CURRENT_USER|windows.CERT_STORE_OPEN_EXISTING_FLAG,
		uintptr(unsafe.Pointer(storeName)),
	)
	if err != nil {
		return 0, fmt.Errorf("%w: opening Windows CurrentUser Root store: %w", errUtils.ErrInvalidConfig, err)
	}
	return store, nil
}
