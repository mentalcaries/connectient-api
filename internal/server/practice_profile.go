package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

const (
	maxLogoSize           = 5 * 1024 * 1024
	maxProfileRequestSize = maxLogoSize + 1024*1024
)

var (
	practiceCodePattern = regexp.MustCompile(`^[a-z0-9-]+$`)
	urlSchemePattern    = regexp.MustCompile(`(?i)^[a-z][a-z\d+.-]*:`)
	domainLabelPattern  = regexp.MustCompile(`(?i)^[a-z\d](?:[a-z\d-]{0,61}[a-z\d])?$`)
	topLevelDomain      = regexp.MustCompile(`(?i)^(?:[a-z]{2,63}|xn--[a-z\d-]{2,59})$`)
)

type PracticeProfileResponse struct {
	ID                   uuid.UUID `json:"id"`
	Name                 string    `json:"name"`
	Logo                 *string   `json:"logo"`
	City                 string    `json:"city"`
	StreetAddress        *string   `json:"street_address"`
	Phone                *string   `json:"phone"`
	Email                *string   `json:"email"`
	Website              *string   `json:"website"`
	PracticeCode         string    `json:"practice_code"`
	Instagram            *string   `json:"instagram"`
	Facebook             *string   `json:"facebook"`
	HasMultipleProviders bool      `json:"has_multiple_providers"`
	PracticeCategory     string    `json:"practice_category"`
	Specialty            *string   `json:"specialty"`
}

type practiceProfileInput struct {
	Name                 string
	StreetAddress        string
	City                 string
	Phone                string
	Email                string
	Website              string
	Facebook             string
	Instagram            string
	Specialty            string
	PracticeCode         string
	HasMultipleProviders bool
	LogoAction           string
	Logo                 []byte
	LogoContentType      string
}

func (s *Server) handlerGetPracticeProfile(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	profile, err := s.DBQuery.GetPracticeProfile(c, *user.PracticeId)
	if err != nil {
		respondPracticeProfileError(c, http.StatusInternalServerError, "Failed to fetch practice profile", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"practice": practiceProfileResponse(profile)})
}

func (s *Server) handlerPatchPracticeCode(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	var body struct {
		PracticeCode string `json:"practiceCode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondPracticeProfileError(c, http.StatusBadRequest, "Practice code is required", nil)
		return
	}
	if message := validatePracticeCode(body.PracticeCode); message != "" {
		respondPracticeProfileError(c, http.StatusBadRequest, message, nil)
		return
	}
	if slices.Contains(RESERVED_CODES, body.PracticeCode) {
		respondPracticeProfileError(c, http.StatusConflict, "This practice code is not available", nil)
		return
	}
	rows, err := s.DBQuery.UpdatePracticeCode(c, db.UpdatePracticeCodeParams{PracticeCode: body.PracticeCode, ID: *user.PracticeId})
	if isUniqueViolation(err) {
		respondPracticeProfileError(c, http.StatusConflict, "This practice code is already taken", nil)
		return
	}
	if err != nil || rows != 1 {
		if err == nil {
			err = errors.New("practice not found")
		}
		respondPracticeProfileError(c, http.StatusInternalServerError, "Failed to update practice code", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handlerPatchPracticeProfile(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxProfileRequestSize)
	input, err := decodePracticeProfile(c.Request)
	if err != nil {
		respondPracticeProfileError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if slices.Contains(RESERVED_CODES, input.PracticeCode) {
		respondPracticeProfileError(c, http.StatusConflict, "This practice code is not available", nil)
		return
	}
	current, err := s.DBQuery.GetPracticeProfile(c, *user.PracticeId)
	if err != nil {
		respondPracticeProfileError(c, http.StatusInternalServerError, "Failed to update practice information", err)
		return
	}

	var newLogoURL *string
	if input.LogoAction != "" {
		if s.storage == nil {
			respondPracticeProfileError(c, http.StatusServiceUnavailable, "Logo storage is not configured", nil)
			return
		}
		if input.LogoAction == "upload" {
			key := fmt.Sprintf("%s/logo_%d", user.PracticeId.String(), time.Now().UnixNano())
			if err := s.storage.Put(c, key, input.LogoContentType, bytes.NewReader(input.Logo), int64(len(input.Logo))); err != nil {
				respondPracticeProfileError(c, http.StatusInternalServerError, "Failed to upload logo", err)
				return
			}
			url := s.storage.PublicURL(key)
			newLogoURL = &url
		}
	}

	params := db.UpdatePracticeProfileParams{
		Name: input.Name, StreetAddress: &input.StreetAddress, City: input.City,
		Phone: &input.Phone, Email: &input.Email, Website: &input.Website,
		PracticeCode: input.PracticeCode, Facebook: &input.Facebook,
		Instagram: &input.Instagram, Specialty: &input.Specialty,
		HasMultipleProviders: input.HasMultipleProviders,
		SetLogo:              input.LogoAction != "", Logo: newLogoURL, ID: *user.PracticeId,
	}
	rows, err := s.DBQuery.UpdatePracticeProfile(c, params)
	if err != nil || rows != 1 {
		if newLogoURL != nil {
			if key, ok := s.storage.OwnedKey(*newLogoURL, user.PracticeId.String()); ok {
				_ = s.storage.Delete(c, key)
			}
		}
		if isUniqueViolation(err) {
			respondPracticeProfileError(c, http.StatusConflict, "This practice code is already taken", nil)
			return
		}
		if err == nil {
			err = errors.New("practice not found")
		}
		respondPracticeProfileError(c, http.StatusInternalServerError, "Failed to update practice information", err)
		return
	}

	if input.LogoAction != "" && current.Logo != nil {
		if key, ok := s.storage.OwnedKey(*current.Logo, user.PracticeId.String()); ok {
			if err := s.storage.Delete(c, key); err != nil {
				log.Printf("failed to delete replaced practice logo %q: %v", key, err)
			}
		}
	}
	response := gin.H{"success": true, "message": "Practice information updated successfully"}
	if input.LogoAction != "" {
		response["newLogoUrl"] = newLogoURL
	}
	c.JSON(http.StatusOK, response)
}

func decodePracticeProfile(request *http.Request) (practiceProfileInput, error) {
	if err := request.ParseMultipartForm(maxProfileRequestSize); err != nil {
		return practiceProfileInput{}, errors.New("Invalid multipart form")
	}
	input := practiceProfileInput{
		Name: request.FormValue("name"), StreetAddress: request.FormValue("streetAddress"),
		City: request.FormValue("city"), Phone: request.FormValue("phone"),
		Email: request.FormValue("email"), Website: request.FormValue("website"),
		Facebook: request.FormValue("facebook"), Instagram: request.FormValue("instagram"),
		Specialty: request.FormValue("specialty"), PracticeCode: request.FormValue("practiceCode"),
		HasMultipleProviders: request.FormValue("has_multiple_providers") == "true",
		LogoAction:           request.FormValue("logoAction"),
	}
	if utf8.RuneCountInString(input.Name) < 4 {
		return practiceProfileInput{}, errors.New("Practice name must be at least 4 characters")
	}
	if utf8.RuneCountInString(input.City) < 4 {
		return practiceProfileInput{}, errors.New("City must be at least 4 characters")
	}
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email {
		return practiceProfileInput{}, errors.New("Please enter a valid email address")
	}
	normalizedWebsite, err := normalizeWebsite(input.Website)
	if err != nil {
		return practiceProfileInput{}, errors.New("Please enter a valid website URL")
	}
	input.Website = normalizedWebsite
	if message := validatePracticeCode(input.PracticeCode); message != "" {
		return practiceProfileInput{}, errors.New(message)
	}
	if input.LogoAction != "" && input.LogoAction != "upload" && input.LogoAction != "remove" {
		return practiceProfileInput{}, errors.New("Invalid logo action")
	}
	if input.LogoAction == "upload" {
		file, header, err := request.FormFile("logoFile")
		if err != nil {
			return practiceProfileInput{}, errors.New("Logo file is required")
		}
		defer file.Close()
		input.Logo, input.LogoContentType, err = readValidatedLogo(file, header)
		if err != nil {
			return practiceProfileInput{}, err
		}
	}
	return input, nil
}

func readValidatedLogo(file multipart.File, header *multipart.FileHeader) ([]byte, string, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxLogoSize+1))
	if err != nil {
		return nil, "", errors.New("Failed to read logo")
	}
	if len(data) == 0 {
		return nil, "", errors.New("Logo file is required")
	}
	if len(data) > maxLogoSize {
		return nil, "", errors.New("File size too large. Maximum size is 5MB")
	}
	allowed := []string{"image/jpeg", "image/png", "image/webp"}
	submitted := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if submitted == "image/jpg" {
		submitted = "image/jpeg"
	}
	detected := http.DetectContentType(data)
	if !slices.Contains(allowed, submitted) || !slices.Contains(allowed, detected) || submitted != detected {
		return nil, "", errors.New("Invalid file type. Accepted: JPEG, PNG, WebP")
	}
	return data, detected, nil
}

func validatePracticeCode(code string) string {
	switch {
	case code == "":
		return "Practice code is required"
	case utf8.RuneCountInString(code) < 4:
		return "Practice code must be at least 4 characters"
	case utf8.RuneCountInString(code) > 30:
		return "Practice code must be 30 characters or less"
	case !practiceCodePattern.MatchString(code):
		return "Practice code can only contain lowercase letters, numbers, and hyphens"
	default:
		return ""
	}
}

func normalizeWebsite(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}

	candidate := trimmed
	if urlSchemePattern.MatchString(candidate) {
		lower := strings.ToLower(candidate)
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return "", errors.New("unsupported website scheme")
		}
	} else {
		candidate = "https://" + candidate
	}

	parsed, err := url.Parse(candidate)
	if err != nil || !validPublicWebsiteDomain(parsed.Hostname()) || parsed.User != nil {
		return "", errors.New("invalid website URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path == "/" && parsed.RawQuery == "" && parsed.Fragment == "" {
		parsed.Path = ""
	}
	return parsed.String(), nil
}

func validPublicWebsiteDomain(hostname string) bool {
	if len(hostname) > 253 || !strings.Contains(hostname, ".") {
		return false
	}
	labels := strings.Split(hostname, ".")
	for _, label := range labels {
		if !domainLabelPattern.MatchString(label) {
			return false
		}
	}
	return topLevelDomain.MatchString(labels[len(labels)-1])
}

func practiceProfileResponse(row db.GetPracticeProfileRow) PracticeProfileResponse {
	return PracticeProfileResponse{
		ID: row.ID, Name: row.Name, Logo: row.Logo, City: row.City,
		StreetAddress: row.StreetAddress, Phone: row.Phone, Email: row.Email,
		Website: row.Website, PracticeCode: row.PracticeCode,
		Instagram: row.Instagram, Facebook: row.Facebook,
		HasMultipleProviders: row.HasMultipleProviders,
		PracticeCategory:     row.PracticeCategory, Specialty: row.Specialty,
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func respondPracticeProfileError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Println(err)
	}
	if status > 499 {
		log.Printf("Responding with a %v error: %s", status, message)
	}
	c.JSON(status, gin.H{"success": false, "error": message})
}
