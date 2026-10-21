package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"vivant/internal/devicekey"
	"vivant/internal/store"
)

// Chaque requête du téléphone porte trois en-têtes :
//
//	X-Vivant-Device     identifiant de l'appareil
//	X-Vivant-Timestamp  heure Unix en secondes
//	X-Vivant-Signature  signature de devicekey.RequestMessage
const deviceClockSkew = 5 * time.Minute

type deviceHandler func(w http.ResponseWriter, r *http.Request, dev store.Device, user store.User, body []byte)

func (s *Server) deviceAuth(h deviceHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		unauthorized := newAPIError(http.StatusUnauthorized, "unauthorized_device", "Appareil non authentifié.")
		devID := r.Header.Get("X-Vivant-Device")
		ts := r.Header.Get("X-Vivant-Timestamp")
		sig := r.Header.Get("X-Vivant-Signature")
		if devID == "" || ts == "" || sig == "" {
			s.writeError(w, unauthorized)
			return
		}
		sec, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			s.writeError(w, unauthorized)
			return
		}
		now := s.Now()
		if d := now.Sub(time.Unix(sec, 0)); d > deviceClockSkew || d < -deviceClockSkew {
			s.writeError(w, newAPIError(http.StatusUnauthorized, "clock_skew", "L'heure du téléphone est incorrecte."))
			return
		}
		dev, ok := s.Store.GetDevice(devID)
		if !ok || dev.RevokedAt != nil {
			s.writeError(w, unauthorized)
			return
		}
		pub, err := devicekey.ParsePublicKey(dev.PublicKey)
		if err != nil {
			s.writeError(w, err)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.writeError(w, newAPIError(http.StatusBadRequest, "invalid_body", "Corps de requête illisible."))
			return
		}
		if devicekey.Verify(pub, devicekey.RequestMessage(r.Method, r.URL.Path, ts, body), sig) != nil {
			s.writeError(w, unauthorized)
			return
		}
		user, ok := s.Store.GetUser(dev.UserID)
		if !ok {
			s.writeError(w, unauthorized)
			return
		}
		s.Store.TouchDevice(dev.ID, now)
		h(w, r, dev, user, body)
	}
}

func (s *Server) handleDeviceMe(w http.ResponseWriter, r *http.Request, dev store.Device, user store.User, _ []byte) {
	type deviceView struct {
		ID         string     `json:"id"`
		Name       string     `json:"name"`
		Current    bool       `json:"current"`
		CreatedAt  time.Time  `json:"created_at"`
		LastSeenAt time.Time  `json:"last_seen_at"`
		RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	}
	var devices []deviceView
	for _, d := range s.Store.UserDevices(user.ID) {
		devices = append(devices, deviceView{d.ID, d.Name, d.ID == dev.ID, d.CreatedAt, d.LastSeenAt, d.RevokedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":     user.ID,
		"identity":    user.Identity,
		"phone":       user.Phone,
		"loa":         user.LoA,
		"verified_at": user.VerifiedAt,
		"devices":     devices,
		"history":     s.Store.AuditForUser(user.ID, 20),
	})
}

type deviceRequestView struct {
	ID             string    `json:"id"`
	ClientID       string    `json:"client_id"`
	ClientName     string    `json:"client_name"`
	Claims         []string  `json:"claims"`
	Challenge      string    `json:"challenge"`
	BindingChoices []string  `json:"binding_choices"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// handleDeviceRequests renvoie les demandes de connexion à valider. En
// production, le téléphone est réveillé par une notification push
// (FCM / APNs) au lieu d'interroger ce point d'accès.
func (s *Server) handleDeviceRequests(w http.ResponseWriter, r *http.Request, _ store.Device, user store.User, _ []byte) {
	out := []deviceRequestView{}
	for _, a := range s.Store.PendingAuthRequests(user.ID, s.Now()) {
		out = append(out, deviceRequestView{
			ID:             a.ID,
			ClientID:       a.ClientID,
			ClientName:     s.clientName(a.ClientID),
			Claims:         describeScopes(a.Scopes),
			Challenge:      a.Challenge,
			BindingChoices: a.BindingChoices,
			CreatedAt:      a.CreatedAt,
			ExpiresAt:      a.ExpiresAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

var errBindingMismatch = newAPIError(http.StatusForbidden, "binding_mismatch",
	"Le code choisi ne correspond pas à celui affiché à l'écran. Par sécurité, la demande a été annulée.")

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request, dev store.Device, user store.User, body []byte) {
	var req struct {
		BindingCode string `json:"binding_code"`
		Signature   string `json:"signature"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeError(w, newAPIError(http.StatusBadRequest, "invalid_json", "Corps de requête JSON invalide."))
		return
	}
	pub, err := devicekey.ParsePublicKey(dev.PublicKey)
	if err != nil {
		s.writeError(w, err)
		return
	}
	now := s.Now()
	var approved store.AuthRequest
	err = s.Store.UpdateAuthRequest(r.PathValue("id"), func(a *store.AuthRequest) error {
		if a.UserID != user.ID {
			return store.ErrNotFound
		}
		if a.Status != store.AuthPendingDevice {
			return newAPIError(http.StatusConflict, "already_handled", "Cette demande a déjà été traitée.")
		}
		if now.After(a.ExpiresAt) {
			a.Status = store.AuthExpired
			return newAPIError(http.StatusGone, "expired", "Cette demande a expiré.")
		}
		// Le code affiché dans le navigateur doit être choisi sur le
		// téléphone : cela empêche d'approuver par réflexe une demande
		// lancée par un fraudeur qui connaît le numéro de la victime.
		if req.BindingCode != a.BindingCode {
			a.Status = store.AuthDenied
			a.CompletedAt = now
			return errBindingMismatch
		}
		if devicekey.Verify(pub, devicekey.ApprovalMessage(a.ID, a.Challenge, req.BindingCode), req.Signature) != nil {
			return newAPIError(http.StatusBadRequest, "invalid_signature", "Signature invalide.")
		}
		a.Status = store.AuthApproved
		a.DeviceID = dev.ID
		a.DeviceSignature = req.Signature
		a.ApprovedAt = now
		a.CompletedAt = now
		a.Code = randomToken(32)
		a.CodeExpiresAt = now.Add(codeTTL)
		approved = *a
		return nil
	})
	if errors.Is(err, errBindingMismatch) {
		if ar, ok := s.Store.GetAuthRequest(r.PathValue("id")); ok {
			s.countPartner(ar.ClientID, func(st *store.PartnerStats) { st.Denied++ })
		}
		s.audit(store.AuditEvent{Type: "auth_binding_mismatch", UserID: user.ID, DeviceID: dev.ID,
			Detail: "Mauvais code choisi : demande annulée"})
	}
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.countPartner(approved.ClientID, func(st *store.PartnerStats) { st.Approved++ })
	s.audit(store.AuditEvent{Type: "auth_approved", UserID: user.ID, DeviceID: dev.ID, ClientID: approved.ClientID,
		Detail: "Connexion à " + s.clientName(approved.ClientID)})
	writeJSON(w, http.StatusOK, map[string]string{"status": string(store.AuthApproved)})
}

func (s *Server) handleDeny(w http.ResponseWriter, r *http.Request, dev store.Device, user store.User, _ []byte) {
	var clientID string
	err := s.Store.UpdateAuthRequest(r.PathValue("id"), func(a *store.AuthRequest) error {
		if a.UserID != user.ID {
			return store.ErrNotFound
		}
		if a.Status != store.AuthPendingDevice {
			return newAPIError(http.StatusConflict, "already_handled", "Cette demande a déjà été traitée.")
		}
		a.Status = store.AuthDenied
		a.CompletedAt = s.Now()
		clientID = a.ClientID
		return nil
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.countPartner(clientID, func(st *store.PartnerStats) { st.Denied++ })
	s.audit(store.AuditEvent{Type: "auth_denied", UserID: user.ID, DeviceID: dev.ID, ClientID: clientID,
		Detail: "Connexion refusée à " + s.clientName(clientID)})
	writeJSON(w, http.StatusOK, map[string]string{"status": string(store.AuthDenied)})
}

func (s *Server) clientName(id string) string {
	if c, ok := s.client(id); ok && c.Name != "" {
		return c.Name
	}
	return id
}

// handleRevoke désactive ce téléphone (suppression du compte sur l'appareil).
// Une nouvelle inscription avec la carte sera nécessaire.
func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request, dev store.Device, user store.User, _ []byte) {
	if err := s.Store.RevokeDevice(dev.ID, s.Now()); err != nil {
		s.writeError(w, err)
		return
	}
	s.audit(store.AuditEvent{Type: "device_revoked", UserID: user.ID, DeviceID: dev.ID,
		Detail: "Téléphone « " + dev.Name + " » désactivé par son titulaire"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
