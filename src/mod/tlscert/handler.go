package tlscert

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/acme"
	"imuslab.com/zoraxy/mod/utils"
)

// Handle cert remove
func (m *Manager) HandleCertRemove(w http.ResponseWriter, r *http.Request) {
	domain, err := utils.PostPara(r, "domain")
	if err != nil {
		utils.SendErrorResponse(w, "invalid domain given")
		return
	}
	err = m.RemoveCert(domain)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
	}
}

// Handle download of the selected certificate
func (m *Manager) HandleCertDownload(w http.ResponseWriter, r *http.Request) {
	// get the certificate name
	certname, err := utils.GetPara(r, "certname")
	if err != nil {
		utils.SendErrorResponse(w, "invalid certname given")
		return
	}
	certname = filepath.Base(certname) //prevent path escape

	// check if the cert exists
	pubKey := filepath.Join(filepath.Join(m.CertStore), certname+".key")
	priKey := filepath.Join(filepath.Join(m.CertStore), certname+".pem")

	if utils.FileExists(pubKey) && utils.FileExists(priKey) {
		//Zip them and serve them via http download
		seeking, _ := utils.GetBool(r, "seek")
		if seeking {
			//This request only check if the key exists. Do not provide download
			utils.SendOK(w)
			return
		}

		//Serve both file in zip
		zipTmpFolder := "./tmp/download"
		os.MkdirAll(zipTmpFolder, 0775)
		zipFileName := filepath.Join(zipTmpFolder, certname+".zip")
		err := utils.ZipFiles(zipFileName, pubKey, priKey)
		if err != nil {
			http.Error(w, "Failed to create zip file", http.StatusInternalServerError)
			return
		}
		defer os.Remove(zipFileName) // Clean up the zip file after serving

		// Serve the zip file
		w.Header().Set("Content-Disposition", "attachment; filename=\""+certname+"_export.zip\"")
		w.Header().Set("Content-Type", "application/zip")
		http.ServeFile(w, r, zipFileName)
	} else {
		//Not both key exists
		utils.SendErrorResponse(w, "invalid key-pairs: private key or public key not found in key store")
		return
	}
}

// Set the selected certificate as the default / fallback certificate
func (m *Manager) SetCertAsDefault(w http.ResponseWriter, r *http.Request) {
	certname, err := utils.PostPara(r, "certname")
	if err != nil {
		utils.SendErrorResponse(w, "invalid certname given")
		return
	}

	//Check if the previous default cert exists. If yes, get its hostname from cert contents
	defaultPubKey := filepath.Join(m.CertStore, "default.key")
	defaultPriKey := filepath.Join(m.CertStore, "default.pem")
	if utils.FileExists(defaultPubKey) && utils.FileExists(defaultPriKey) {
		//Move the existing default cert to its original name
		certBytes, err := os.ReadFile(defaultPriKey)
		if err == nil {
			block, _ := pem.Decode(certBytes)
			if block != nil {
				cert, err := x509.ParseCertificate(block.Bytes)
				if err == nil {
					os.Rename(defaultPubKey, filepath.Join(m.CertStore, domainToFilename(cert.Subject.CommonName, "key")))
					os.Rename(defaultPriKey, filepath.Join(m.CertStore, domainToFilename(cert.Subject.CommonName, "pem")))
				}
			}
		}
	}

	//Check if the cert exists
	certname = filepath.Base(certname) //prevent path escape
	pubKey := filepath.Join(filepath.Join(m.CertStore), certname+".key")
	priKey := filepath.Join(filepath.Join(m.CertStore), certname+".pem")
	if utils.FileExists(pubKey) && utils.FileExists(priKey) {
		os.Rename(pubKey, filepath.Join(m.CertStore, "default.key"))
		os.Rename(priKey, filepath.Join(m.CertStore, "default.pem"))
		utils.SendOK(w)

		//Update cert list
		m.UpdateLoadedCertList()

	} else {
		utils.SendErrorResponse(w, "invalid key-pairs: private key or public key not found in key store")
		return
	}
}

// Handle upload of the certificate
func (m *Manager) HandleCertUpload(w http.ResponseWriter, r *http.Request) {
	// check if request method is POST
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// get the key type
	keytype, err := utils.GetPara(r, "ktype")
	overWriteFilename := ""
	if err != nil {
		http.Error(w, "Not defined key type (pub / pri)", http.StatusBadRequest)
		return
	}

	// get the domain
	domain, err := utils.GetPara(r, "domain")
	if err != nil {
		//Assume localhost
		domain = "default"
	}

	switch keytype {
	case "pub":
		overWriteFilename = domain + ".pem"
	case "pri":
		overWriteFilename = domain + ".key"
	default:
		http.Error(w, "Not supported keytype: "+keytype, http.StatusBadRequest)
		return
	}

	// parse multipart form data
	err = r.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	// get file from form data
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// create file in upload directory
	// Read file contents for validation
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}

	os.MkdirAll(m.CertStore, 0775)
	f, err := os.Create(filepath.Join(m.CertStore, overWriteFilename))
	if err != nil {
		http.Error(w, "Failed to create file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// copy file contents to destination file
	_, err = io.Copy(f, file)
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	_, err = f.Write(fileBytes)
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	//Update cert list
	m.UpdateLoadedCertList()

	// send response
	fmt.Fprintln(w, "File upload successful!")
}

// List all certificates and map all their domains to the cert filename
func (m *Manager) HandleListDomains(w http.ResponseWriter, r *http.Request) {
	filenames, err := os.ReadDir(m.CertStore)

	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	certnameToDomainMap := map[string]string{}
	for _, filename := range filenames {
		if filename.IsDir() {
			continue
		}
		certFilepath := filepath.Join(m.CertStore, filename.Name())

		certBtyes, err := os.ReadFile(certFilepath)
		if err != nil {
			// Unable to load this file
			m.Logger.PrintAndLog("TLS", "Unable to load certificate: "+certFilepath, err)
			continue
		} else {
			// Cert loaded. Check its expiry time
			block, _ := pem.Decode(certBtyes)
			if block != nil {
				cert, err := x509.ParseCertificate(block.Bytes)
				if err == nil {
					certname := strings.TrimSuffix(filepath.Base(certFilepath), filepath.Ext(certFilepath))
					for _, dnsName := range cert.DNSNames {
						certnameToDomainMap[dnsName] = certname
					}
					certnameToDomainMap[cert.Subject.CommonName] = certname
				}
			}
		}
	}

	requireCompact, _ := utils.GetPara(r, "compact")
	if requireCompact == "true" {
		result := make(map[string][]string)

		for key, value := range certnameToDomainMap {
			if _, ok := result[value]; !ok {
				result[value] = make([]string, 0)
			}

			result[value] = append(result[value], key)
		}

		js, _ := json.Marshal(result)
		utils.SendJSONResponse(w, string(js))
		return
	}

	js, _ := json.Marshal(certnameToDomainMap)
	utils.SendJSONResponse(w, string(js))
}

// Return a list of domains where the certificates covers
func (m *Manager) HandleListCertificate(w http.ResponseWriter, r *http.Request) {
	filenames, err := m.ListCertDomains()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	showDate, _ := utils.GetBool(r, "date")
	if showDate {
		type CertInfo struct {
			Domain           string // Domain name of the certificate
			Filename         string // Filename that stores the certificate
			LastModifiedDate string
			ExpireDate       string
			RemainingDays    int
			UseDNS           bool // Whether this cert is obtained via DNS challenge
			IsFallback       bool // Whether this cert is the fallback/default cert
		}

		results := []*CertInfo{}

		for _, filename := range filenames {
			certFilepath := filepath.Join(m.CertStore, filename+".pem")
			//keyFilepath := filepath.Join(tlsCertManager.CertStore, filename+".key")
			fileInfo, err := os.Stat(certFilepath)
			if err != nil {
				utils.SendErrorResponse(w, "invalid domain certificate discovered: "+filename)
				return
			}
			modifiedTime := fileInfo.ModTime().Format("2006-01-02 15:04:05")

			certExpireTime := "Unknown"
			certBtyes, err := os.ReadFile(certFilepath)
			expiredIn := 0
			if err != nil {
				//Unable to load this file
				continue
			} else {
				//Cert loaded. Check its expire time
				block, _ := pem.Decode(certBtyes)
				if block != nil {
					cert, err := x509.ParseCertificate(block.Bytes)
					if err == nil {
						certExpireTime = cert.NotAfter.Format("2006-01-02 15:04:05")

						duration := time.Until(cert.NotAfter)

						// Convert the duration to days
						expiredIn = int(duration.Hours() / 24)
					}
				}
			}
			certInfoFilename := filepath.Join(m.CertStore, filename+".json")
			useDNSValidation := false                                //Default to false for HTTP TLS certificates
			certInfo, err := acme.LoadCertInfoJSON(certInfoFilename) //Note: Not all certs have info json
			if err == nil {
				useDNSValidation = certInfo.UseDNS
			}

			certDomain := ""
			block, _ := pem.Decode(certBtyes)
			if block != nil {
				cert, err := x509.ParseCertificate(block.Bytes)
				if err == nil {
					certDomain = cert.Subject.CommonName
				}
			}

			thisCertInfo := CertInfo{
				Domain:           certDomain,
				Filename:         filename,
				LastModifiedDate: modifiedTime,
				ExpireDate:       certExpireTime,
				RemainingDays:    expiredIn,
				UseDNS:           useDNSValidation,
				IsFallback:       (filename == "default"), //TODO: figure out a better implementation
			}

			results = append(results, &thisCertInfo)
		}

		// convert ExpireDate to date object and sort asc
		sort.Slice(results, func(i, j int) bool {
			date1, _ := time.Parse("2006-01-02 15:04:05", results[i].ExpireDate)
			date2, _ := time.Parse("2006-01-02 15:04:05", results[j].ExpireDate)
			return date1.Before(date2)
		})

		js, _ := json.Marshal(results)
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
		return
	}

	response, err := json.Marshal(filenames)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(response)
}

// Check if the default certificates is correctly setup
func (m *Manager) HandleDefaultCertCheck(w http.ResponseWriter, r *http.Request) {
	type CheckResult struct {
		DefaultPubExists bool
		DefaultPriExists bool
	}

	pub, pri := m.DefaultCertExistsSep()
	js, _ := json.Marshal(CheckResult{
		pub,
		pri,
	})

	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandleSelfSignCertGenerate(w http.ResponseWriter, r *http.Request) {
	// Get the common name from the request
	cn, err := utils.GetPara(r, "cn")
	if err != nil {
		utils.SendErrorResponse(w, "Common name not provided")
		return
	}

	domains, err := utils.PostPara(r, "domains")
	if err != nil {
		//No alias domains provided, use the common name as the only domain
		domains = "[]"
	}

	SANs := []string{}
	if err := json.Unmarshal([]byte(domains), &SANs); err != nil {
		utils.SendErrorResponse(w, "Invalid domains format: "+err.Error())
		return
	}
	//SANs = append([]string{cn}, SANs...)
	priKeyFilename := domainToFilename(cn, ".key")
	pubKeyFilename := domainToFilename(cn, ".pem")

	// Generate self-signed certificate
	err = m.GenerateSelfSignedCertificate(cn, SANs, pubKeyFilename, priKeyFilename)
	if err != nil {
		utils.SendErrorResponse(w, "Failed to generate self-signed certificate: "+err.Error())
		return
	}

	//Update the certificate store
	err = m.UpdateLoadedCertList()
	if err != nil {
		utils.SendErrorResponse(w, "Failed to update certificate store: "+err.Error())
		return
	}
	utils.SendOK(w)
}

// Extract the common name from a PEM encoded certificate
func (m *Manager) HandleGetCertCommonName(w http.ResponseWriter, r *http.Request) {
	certContents, err := utils.PostPara(r, "cert")
	if err != nil {
		utils.SendErrorResponse(w, "Certificate content not provided")
		return
	}
	block, _ := pem.Decode([]byte(certContents))
	if block == nil {
		utils.SendErrorResponse(w, "Failed to decode PEM block")
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		utils.SendErrorResponse(w, "Failed to parse certificate: "+err.Error())
		return
	}

	js, _ := json.Marshal(cert.Subject.CommonName)
	utils.SendJSONResponse(w, string(js))
}
