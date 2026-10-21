// Commande device-sim : simule le téléphone et l'application Vivant en
// attendant l'application mobile.
//
//	device-sim enroll -nin 2751199000123 -phone 771234567
//	device-sim watch     # attend les demandes de connexion et les valide
//	device-sim me        # affiche l'identité et l'historique
//
// La clé privée est enregistrée dans un fichier ; sur un vrai téléphone,
// elle reste dans la puce sécurisée et ne peut pas être exportée.
package main

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"vivant/internal/devicekey"
	"vivant/internal/registry"
)

type deviceState struct {
	Server     string `json:"server"`
	DeviceID   string `json:"device_id"`
	UserID     string `json:"user_id"`
	PrivateKey string `json:"private_key"`
	key        *ecdsa.PrivateKey
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "enroll":
		err = cmdEnroll(os.Args[2:])
	case "watch":
		err = cmdWatch(os.Args[2:])
	case "me":
		err = cmdMe(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Simulateur de téléphone Vivant

  device-sim enroll -nin <NIN> -phone <numéro> [-name <appareil>]
  device-sim watch
  device-sim me

Options communes : -server http://localhost:8080  -state .vivant-device.json`)
	os.Exit(2)
}

func commonFlags(fs *flag.FlagSet) (server, state *string) {
	server = fs.String("server", "http://localhost:8080", "URL du serveur Vivant")
	state = fs.String("state", ".vivant-device.json", "fichier d'état du téléphone simulé")
	return
}

// --- HTTP ---

type apiErr struct {
	Code    string `json:"error"`
	Message string `json:"message"`
}

func decodeResponse(resp *http.Response, out any) error {
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e apiErr
		if json.Unmarshal(raw, &e) == nil && e.Message != "" {
			return fmt.Errorf("%s (%s)", e.Message, e.Code)
		}
		return fmt.Errorf("statut HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func postJSON(url string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	return decodeResponse(resp, out)
}

// signed envoie une requête signée par la clé de l'appareil.
func (d *deviceState) signed(method, path string, body, out any) error {
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return err
		}
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig, err := devicekey.Sign(d.key, devicekey.RequestMessage(method, path, ts, raw))
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, d.Server+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vivant-Device", d.DeviceID)
	req.Header.Set("X-Vivant-Timestamp", ts)
	req.Header.Set("X-Vivant-Signature", sig)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	return decodeResponse(resp, out)
}

// --- État ---

func loadState(path string) (*deviceState, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("aucun téléphone inscrit (%s introuvable) : lancez d'abord « device-sim enroll »", path)
	}
	if err != nil {
		return nil, err
	}
	var d deviceState
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(d.PrivateKey))
	if block == nil {
		return nil, errors.New("clé privée illisible")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("clé ECDSA attendue")
	}
	d.key = key
	return &d, nil
}

func (d *deviceState) save(path string) error {
	der, err := x509.MarshalPKCS8PrivateKey(d.key)
	if err != nil {
		return err
	}
	d.PrivateKey = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// --- Commandes ---

func cmdEnroll(args []string) error {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	server, statePath := commonFlags(fs)
	nin := fs.String("nin", "", "numéro d'identification national (13 chiffres)")
	tel := fs.String("phone", "", "numéro de téléphone enregistré à votre nom")
	name := fs.String("name", "Téléphone simulé", "nom de l'appareil")
	_ = fs.Parse(args)
	if *nin == "" || *tel == "" {
		return errors.New("-nin et -phone sont obligatoires")
	}
	base := strings.TrimRight(*server, "/")

	fmt.Println("1/5  Vérification du NIN et du numéro de téléphone…")
	var start struct {
		ID     string `json:"enrollment_id"`
		DevOTP string `json:"dev_otp"`
	}
	if err := postJSON(base+"/api/v1/enrollments", map[string]string{"nin": *nin, "phone": *tel}, &start); err != nil {
		return err
	}
	code := start.DevOTP
	if code == "" {
		fmt.Print("     Code reçu par SMS : ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		code = strings.TrimSpace(line)
	} else {
		fmt.Printf("     SMS reçu (mode développement) : %s\n", code)
	}

	fmt.Println("2/5  Validation du code SMS…")
	if err := postJSON(base+"/api/v1/enrollments/"+start.ID+"/otp", map[string]string{"code": code}, nil); err != nil {
		return err
	}

	fmt.Println("3/5  Lecture de la carte d'identité (simulée à partir du registre de test)…")
	card, err := simulatedCard(*nin)
	if err != nil {
		return err
	}
	for _, l := range strings.Split(card, "\n") {
		fmt.Println("     " + l)
	}
	if err := postJSON(base+"/api/v1/enrollments/"+start.ID+"/document", map[string]string{"mrz": card}, nil); err != nil {
		return err
	}

	fmt.Println("4/5  Selfie et détection du vivant (simulés)…")
	selfie := make([]byte, 2048)
	_, _ = rand.Read(selfie)
	if err := postJSON(base+"/api/v1/enrollments/"+start.ID+"/selfie",
		map[string]string{"selfie": base64.StdEncoding.EncodeToString(selfie)}, nil); err != nil {
		return err
	}

	fmt.Println("5/5  Création de la clé sécurisée du téléphone…")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	pub, err := devicekey.EncodePublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	sig, err := devicekey.Sign(key, devicekey.EnrollmentMessage(start.ID))
	if err != nil {
		return err
	}
	var done struct {
		UserID   string `json:"user_id"`
		DeviceID string `json:"device_id"`
		Identity struct {
			FamilyName string `json:"family_name"`
			GivenNames string `json:"given_names"`
		} `json:"identity"`
	}
	if err := postJSON(base+"/api/v1/enrollments/"+start.ID+"/complete",
		map[string]string{"public_key": pub, "device_name": *name, "signature": sig}, &done); err != nil {
		return err
	}
	st := &deviceState{Server: base, DeviceID: done.DeviceID, UserID: done.UserID, key: key}
	if err := st.save(*statePath); err != nil {
		return err
	}
	fmt.Printf("\n✓ Bienvenue %s %s ! Votre identité Vivant est activée sur ce téléphone.\n",
		done.Identity.GivenNames, done.Identity.FamilyName)
	fmt.Println("  Lancez « device-sim watch » pour recevoir les demandes de connexion.")
	return nil
}

func simulatedCard(nin string) (string, error) {
	cs, err := registry.Fixtures()
	if err != nil {
		return "", err
	}
	for _, c := range cs {
		if c.NIN == nin {
			return registry.SimulatedCardMRZ(c)
		}
	}
	return "", fmt.Errorf("aucune carte de test pour le NIN %s", nin)
}

type pendingRequest struct {
	ID             string   `json:"id"`
	ClientName     string   `json:"client_name"`
	Claims         []string `json:"claims"`
	Challenge      string   `json:"challenge"`
	BindingChoices []string `json:"binding_choices"`
}

func cmdWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	_, statePath := commonFlags(fs)
	_ = fs.Parse(args)
	st, err := loadState(*statePath)
	if err != nil {
		return err
	}
	in := bufio.NewReader(os.Stdin)
	seen := map[string]bool{}
	fmt.Println("En attente de demandes de connexion… (Ctrl+C pour quitter)")
	for {
		var reqs []pendingRequest
		if err := st.signed("GET", "/api/v1/device/requests", nil, &reqs); err != nil {
			fmt.Fprintln(os.Stderr, "✗", err)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, rq := range reqs {
			if seen[rq.ID] {
				continue
			}
			seen[rq.ID] = true
			fmt.Printf("\n┌ Demande de connexion : %s\n", rq.ClientName)
			fmt.Println("│ Données demandées :")
			for _, c := range rq.Claims {
				fmt.Println("│   •", c)
			}
			fmt.Printf("│ Quel code s'affiche sur l'écran ?  [ %s ]\n", strings.Join(rq.BindingChoices, " ]  [ "))
			fmt.Print("└ Tapez le code pour confirmer (Entrée seule pour refuser) : ")
			line, _ := in.ReadString('\n')
			choice := strings.TrimSpace(line)
			if choice == "" {
				if err := st.signed("POST", "/api/v1/device/requests/"+rq.ID+"/deny", map[string]string{}, nil); err != nil {
					fmt.Println("  ✗", err)
				} else {
					fmt.Println("  Demande refusée.")
				}
				continue
			}
			fmt.Println("  (déverrouillage de la clé par PIN ou empreinte : simulé)")
			sig, err := devicekey.Sign(st.key, devicekey.ApprovalMessage(rq.ID, rq.Challenge, choice))
			if err != nil {
				return err
			}
			if err := st.signed("POST", "/api/v1/device/requests/"+rq.ID+"/approve",
				map[string]string{"binding_code": choice, "signature": sig}, nil); err != nil {
				fmt.Println("  ✗", err)
			} else {
				fmt.Println("  ✓ Connexion approuvée.")
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func cmdMe(args []string) error {
	fs := flag.NewFlagSet("me", flag.ExitOnError)
	_, statePath := commonFlags(fs)
	_ = fs.Parse(args)
	st, err := loadState(*statePath)
	if err != nil {
		return err
	}
	var me struct {
		Identity struct {
			NIN        string `json:"nin"`
			FamilyName string `json:"family_name"`
			GivenNames string `json:"given_names"`
			BirthDate  string `json:"birth_date"`
			BirthPlace string `json:"birth_place"`
		} `json:"identity"`
		Phone   string `json:"phone"`
		LoA     string `json:"loa"`
		History []struct {
			At     time.Time `json:"at"`
			Detail string    `json:"detail"`
		} `json:"history"`
	}
	if err := st.signed("GET", "/api/v1/device/me", nil, &me); err != nil {
		return err
	}
	id := me.Identity
	fmt.Printf("%s %s\nNé(e) le %s à %s\nNIN : %s\nTéléphone : %s\nNiveau : %s\n\nHistorique :\n",
		id.GivenNames, strings.ToUpper(id.FamilyName), id.BirthDate, id.BirthPlace, id.NIN, me.Phone, me.LoA)
	for _, h := range me.History {
		fmt.Printf("  %s  %s\n", h.At.Local().Format("02/01/2006 15:04"), h.Detail)
	}
	return nil
}
