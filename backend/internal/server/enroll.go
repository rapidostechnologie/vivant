package server

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"vivant/internal/devicekey"
	"vivant/internal/mrz"
	"vivant/internal/phone"
	"vivant/internal/registry"
	"vivant/internal/store"
)

// Parcours d'inscription :
//
//  1. Numéro de téléphone : le registre des SIM donne le titulaire de la
//     puce (le NIN, s'il est fourni, doit correspondre). Un code est envoyé
//     par SMS.
//  2. Code SMS : prouve la possession de la puce.
//  3. Carte d'identité (MRZ) : les données doivent correspondre au registre.
//  4. Selfie : détection du vivant et comparaison avec la photo de référence.
//  5. Clé de l'appareil : le téléphone envoie sa clé publique et prouve
//     qu'il détient la clé privée.
const (
	enrollmentTTL      = 30 * time.Minute
	otpTTL             = 5 * time.Minute
	maxOTPAttempts     = 5
	maxFailures        = 5
	faceMatchThreshold = 0.80
	loaSubstantial     = "urn:vivant:loa:substantial"
)

type enrollmentView struct {
	EnrollmentID string               `json:"enrollment_id"`
	Step         store.EnrollmentStep `json:"step"`
	ExpiresAt    time.Time            `json:"expires_at"`
	DevOTP       string               `json:"dev_otp,omitempty"`
}

func viewOf(e store.Enrollment) enrollmentView {
	return enrollmentView{EnrollmentID: e.ID, Step: e.Step, ExpiresAt: e.ExpiresAt}
}

func otpHash(enrollmentID, otp string) string {
	return hashToken(enrollmentID + ":" + otp)
}

// fail compte un échec ; au-delà de maxFailures l'inscription est annulée.
func fail(e *store.Enrollment, now time.Time) {
	e.Failures++
	if e.Failures >= maxFailures {
		e.ExpiresAt = now
	}
}

// updateEnrollment met à jour une inscription après avoir vérifié qu'elle
// n'a pas expiré et qu'elle est à l'étape attendue.
func (s *Server) updateEnrollment(id string, step store.EnrollmentStep, fn func(*store.Enrollment) error) (store.Enrollment, error) {
	var out store.Enrollment
	err := s.Store.UpdateEnrollment(id, func(e *store.Enrollment) error {
		if s.Now().After(e.ExpiresAt) {
			return newAPIError(http.StatusGone, "enrollment_expired", "L'inscription a expiré. Veuillez recommencer.")
		}
		if e.Step != step {
			return newAPIError(http.StatusConflict, "wrong_step", fmt.Sprintf("Étape en cours : %s.", e.Step))
		}
		err := fn(e)
		out = *e
		return err
	})
	return out, err
}

func (s *Server) handleEnrollStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NIN   string `json:"nin"`
		Phone string `json:"phone"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	tel, err := phone.Normalize(req.Phone)
	if err != nil {
		s.writeError(w, newAPIError(http.StatusBadRequest, "invalid_phone", "Numéro de téléphone invalide."))
		return
	}
	// Le NIN est facultatif : le registre des abonnés de l'opérateur donne le
	// titulaire de la puce, et la carte d'identité le confirme ensuite.
	var citizen registry.Citizen
	if req.NIN != "" {
		nin := phone.Digits(req.NIN)
		if len(nin) != 13 {
			s.writeError(w, newAPIError(http.StatusBadRequest, "invalid_nin", "Le NIN doit comporter 13 chiffres."))
			return
		}
		citizen, err = s.Registry.LookupByNIN(r.Context(), nin)
		if err == nil && citizen.SIMPhone != tel {
			s.writeError(w, newAPIError(http.StatusForbidden, "phone_mismatch",
				"Ce numéro n'est pas enregistré au nom du titulaire de ce NIN. Utilisez la puce enregistrée à votre nom."))
			return
		}
	} else {
		citizen, err = s.Registry.LookupByPhone(r.Context(), tel)
	}
	if errors.Is(err, registry.ErrNotFound) {
		s.writeError(w, newAPIError(http.StatusNotFound, "holder_not_found",
			"Ce numéro n'est enregistré au nom de personne. Utilisez la puce enregistrée à votre nom."))
		return
	}
	if err != nil {
		s.writeError(w, err)
		return
	}
	nin := citizen.NIN

	now := s.Now()
	otp := randomDigits(6)
	e := store.Enrollment{
		ID:           "enr_" + randomToken(16),
		NIN:          nin,
		Phone:        tel,
		Step:         store.StepOTP,
		OTPExpiresAt: now.Add(otpTTL),
		CreatedAt:    now,
		ExpiresAt:    now.Add(enrollmentTTL),
	}
	e.OTPHash = otpHash(e.ID, otp)
	s.Store.CreateEnrollment(e)

	msg := fmt.Sprintf("%s : votre code de vérification est %s. Ne le communiquez à personne.", s.cfg.AppName, otp)
	if err := s.SMS.Send(r.Context(), tel, msg); err != nil {
		s.Logger.Error("envoi SMS", "err", err)
		s.writeError(w, newAPIError(http.StatusBadGateway, "sms_failed", "Impossible d'envoyer le SMS. Réessayez."))
		return
	}
	view := viewOf(e)
	if s.cfg.DevMode {
		view.DevOTP = otp
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *Server) handleEnrollOTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	now := s.Now()
	e, err := s.updateEnrollment(r.PathValue("id"), store.StepOTP, func(e *store.Enrollment) error {
		if e.OTPAttempts >= maxOTPAttempts {
			return newAPIError(http.StatusTooManyRequests, "too_many_attempts", "Trop de tentatives. Recommencez l'inscription.")
		}
		if now.After(e.OTPExpiresAt) {
			return newAPIError(http.StatusGone, "otp_expired", "Le code a expiré. Recommencez l'inscription.")
		}
		e.OTPAttempts++
		if subtle.ConstantTimeCompare([]byte(otpHash(e.ID, strings.TrimSpace(req.Code))), []byte(e.OTPHash)) != 1 {
			return newAPIError(http.StatusBadRequest, "invalid_otp", "Code incorrect.")
		}
		e.OTPHash = ""
		e.Step = store.StepDocument
		return nil
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOf(e))
}

func (s *Server) handleEnrollDocument(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MRZ string `json:"mrz"`
		// Photo recto de la carte : conservée pour contrôle manuel en cas de
		// litige (non exploitée par le prototype).
		DocumentImage string `json:"document_image,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	id := r.PathValue("id")
	e, ok := s.Store.GetEnrollment(id)
	if !ok {
		s.writeError(w, store.ErrNotFound)
		return
	}
	now := s.Now()

	// Les appels externes se font hors verrou ; l'étape est revérifiée ensuite.
	var docErr error
	var citizen registry.Citizen
	card, err := mrz.ParseTD1(req.MRZ, now)
	if err != nil {
		docErr = newAPIError(http.StatusUnprocessableEntity, "invalid_mrz",
			"Lecture de la carte impossible ("+err.Error()+"). Reprenez la photo du verso.")
	} else if citizen, err = s.Registry.LookupByNIN(r.Context(), e.NIN); err != nil {
		s.writeError(w, err)
		return
	} else {
		docErr = s.checkDocument(card, citizen, now)
	}

	e, err = s.updateEnrollment(id, store.StepDocument, func(e *store.Enrollment) error {
		if docErr != nil {
			fail(e, now)
			return docErr
		}
		e.Identity = store.Identity{
			NIN:            citizen.NIN,
			FamilyName:     citizen.FamilyName,
			GivenNames:     citizen.GivenNames,
			BirthDate:      citizen.BirthDate,
			BirthPlace:     citizen.BirthPlace,
			Sex:            citizen.Sex,
			Nationality:    citizen.Nationality,
			Address:        citizen.Address,
			DocumentNumber: citizen.DocumentNumber,
		}
		e.Step = store.StepSelfie
		return nil
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOf(e))
}

// checkDocument vérifie que la carte présentée est valide et appartient bien
// au titulaire du NIN.
func (s *Server) checkDocument(card *mrz.TD1, c registry.Citizen, now time.Time) error {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if card.ExpiryDate.Before(today) {
		return newAPIError(http.StatusUnprocessableEntity, "document_expired",
			fmt.Sprintf("Votre carte d'identité a expiré le %s. Renouvelez-la avant de vous inscrire.",
				card.ExpiryDate.Format("02/01/2006")))
	}
	var mismatches []string
	if card.DocumentNumber != c.DocumentNumber {
		mismatches = append(mismatches, "numéro de carte")
	}
	if card.BirthDate.Format(time.DateOnly) != c.BirthDate {
		mismatches = append(mismatches, "date de naissance")
	}
	if card.Sex != c.Sex {
		mismatches = append(mismatches, "sexe")
	}
	if card.Nationality != c.Nationality {
		mismatches = append(mismatches, "nationalité")
	}
	if !mrz.NamesMatch(c.FamilyName, card.Surname) {
		mismatches = append(mismatches, "nom")
	}
	if len(mismatches) > 0 {
		// Le détail reste dans les journaux pour ne pas guider un fraudeur.
		s.Logger.Warn("carte non concordante", "nin", c.NIN, "champs", strings.Join(mismatches, ", "))
		return newAPIError(http.StatusUnprocessableEntity, "document_mismatch",
			"Cette carte ne correspond pas au registre national pour ce NIN.")
	}
	return nil
}

func (s *Server) handleEnrollSelfie(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Selfie string `json:"selfie"` // image JPEG en base64
	}
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	selfie, err := base64.StdEncoding.DecodeString(req.Selfie)
	if err != nil {
		s.writeError(w, newAPIError(http.StatusBadRequest, "invalid_selfie", "Image invalide."))
		return
	}
	id := r.PathValue("id")
	e, ok := s.Store.GetEnrollment(id)
	if !ok {
		s.writeError(w, store.ErrNotFound)
		return
	}
	if e.Step != store.StepSelfie {
		s.writeError(w, newAPIError(http.StatusConflict, "wrong_step", fmt.Sprintf("Étape en cours : %s.", e.Step)))
		return
	}
	res, err := s.Faces.Verify(r.Context(), e.NIN, selfie)
	if err != nil {
		s.writeError(w, err)
		return
	}
	now := s.Now()
	e, err = s.updateEnrollment(id, store.StepSelfie, func(e *store.Enrollment) error {
		if !res.Live {
			fail(e, now)
			return newAPIError(http.StatusUnprocessableEntity, "liveness_failed",
				"Vérification du vivant échouée. Placez-vous dans un endroit bien éclairé et suivez les instructions.")
		}
		if res.Score < faceMatchThreshold {
			fail(e, now)
			return newAPIError(http.StatusUnprocessableEntity, "face_mismatch",
				"Votre visage ne correspond pas à la photo de référence.")
		}
		e.FaceScore = res.Score
		e.Step = store.StepDevice
		return nil
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOf(e))
}

func (s *Server) handleEnrollComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicKey  string `json:"public_key"`
		DeviceName string `json:"device_name"`
		Signature  string `json:"signature"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	id := r.PathValue("id")
	pub, err := devicekey.ParsePublicKey(req.PublicKey)
	if err != nil {
		s.writeError(w, newAPIError(http.StatusBadRequest, "invalid_public_key", "Clé publique invalide (ECDSA P-256 attendue)."))
		return
	}
	if err := devicekey.Verify(pub, devicekey.EnrollmentMessage(id), req.Signature); err != nil {
		s.writeError(w, newAPIError(http.StatusBadRequest, "invalid_signature", "Preuve de possession de la clé invalide."))
		return
	}
	encodedKey, err := devicekey.EncodePublicKey(pub)
	if err != nil {
		s.writeError(w, err)
		return
	}
	e, err := s.updateEnrollment(id, store.StepDevice, func(e *store.Enrollment) error {
		e.Step = store.StepDone
		return nil
	})
	if err != nil {
		s.writeError(w, err)
		return
	}

	now := s.Now()
	user, exists := s.Store.UserByNIN(e.NIN)
	if exists {
		// Nouveau téléphone (perte, vol, changement) : les anciens appareils
		// sont révoqués, un seul appareil actif par personne.
		n, err := s.Store.RevokeUserDevices(user.ID, now)
		if err != nil {
			s.writeError(w, err)
			return
		}
		if n > 0 {
			s.audit(store.AuditEvent{Type: "devices_revoked", UserID: user.ID,
				Detail: fmt.Sprintf("%d appareil(s) révoqué(s) suite à une nouvelle inscription", n)})
		}
	} else {
		user = store.User{ID: "usr_" + randomToken(12), CreatedAt: now}
	}
	user.Identity = e.Identity
	user.Phone = e.Phone
	user.LoA = loaSubstantial
	user.FaceScore = e.FaceScore
	user.VerifiedAt = now
	if err := s.Store.SaveUser(user); err != nil {
		s.writeError(w, err)
		return
	}

	name := strings.TrimSpace(req.DeviceName)
	if name == "" {
		name = "Téléphone"
	}
	if utf8.RuneCountInString(name) > 64 {
		name = string([]rune(name)[:64])
	}
	dev := store.Device{
		ID:         "dev_" + randomToken(12),
		UserID:     user.ID,
		Name:       name,
		PublicKey:  encodedKey,
		CreatedAt:  now,
		LastSeenAt: now,
	}
	if err := s.Store.AddDevice(dev); err != nil {
		s.writeError(w, err)
		return
	}
	s.audit(store.AuditEvent{Type: "enrolled", UserID: user.ID, DeviceID: dev.ID,
		Detail: "Identité vérifiée et téléphone « " + name + " » activé"})

	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id":   user.ID,
		"device_id": dev.ID,
		"loa":       user.LoA,
		"identity": map[string]string{
			"family_name": user.Identity.FamilyName,
			"given_names": user.Identity.GivenNames,
			"birth_date":  user.Identity.BirthDate,
		},
	})
}

// handleDevCard renvoie la MRZ de la carte du titulaire (mode développement
// uniquement) : elle remplace la caméra pour tester l'application.
func (s *Server) handleDevCard(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.DevMode {
		s.writeError(w, store.ErrNotFound)
		return
	}
	e, ok := s.Store.GetEnrollment(r.PathValue("id"))
	if !ok {
		s.writeError(w, store.ErrNotFound)
		return
	}
	c, err := s.Registry.LookupByNIN(r.Context(), e.NIN)
	if err != nil {
		s.writeError(w, err)
		return
	}
	card, err := registry.SimulatedCardMRZ(c)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"mrz": card})
}
