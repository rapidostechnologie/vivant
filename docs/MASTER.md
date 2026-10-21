# Vivant — document maître

> Le document de référence pour toute personne qui travaille sur Vivant : l'idée, le produit, l'architecture, le code et la suite.
> À lire en premier. À tenir à jour à chaque changement important.

## Sommaire

1. [L'idée en une minute](#1-lidée-en-une-minute)
2. [Le problème](#2-le-problème)
3. [La solution](#3-la-solution)
4. [Pour qui](#4-pour-qui)
5. [Comment ça marche](#5-comment-ça-marche)
6. [Principes du produit](#6-principes-du-produit)
7. [Architecture](#7-architecture)
8. [Carte du code](#8-carte-du-code)
9. [Modèle de données](#9-modèle-de-données)
10. [Les API](#10-les-api)
11. [Sécurité](#11-sécurité)
12. [Ce qui est réel, ce qui est simulé](#12-ce-qui-est-réel-ce-qui-est-simulé)
13. [Travailler sur le projet](#13-travailler-sur-le-projet)
14. [Design](#14-design)
15. [Feuille de route](#15-feuille-de-route)
16. [Cadre légal et partenariats](#16-cadre-légal-et-partenariats)
17. [Glossaire](#17-glossaire)

---

## 1. L'idée en une minute

**Vivant est une identité numérique vérifiée, sur le téléphone, pour l'Afrique.**

Une personne s'inscrit une seule fois : son numéro de téléphone, sa carte d'identité et son visage sont vérifiés. Ensuite, quand un site (administration, banque, assurance…) veut savoir qui elle est, elle confirme sur son téléphone en un toucher. Le site reçoit une identité **certaine**, et non une simple adresse e-mail.

Le modèle s'inspire d'itsme en Belgique, mais il est pensé pour les réalités africaines : téléphones d'entrée de gamme, connexions instables, cartes d'identité différentes selon les pays, et utilisateurs peu habitués au numérique.

## 2. Le problème

Exemple réel : sur un portail de démarches en ligne comme e-senegal.sn, **n'importe qui peut créer un compte et demander un acte d'état civil au nom d'une autre personne**. Le compte prouve qu'on possède une adresse e-mail, pas qu'on est la personne.

Conséquences :
- usurpation d'identité et fraude documentaire ;
- les banques et assurances obligent à venir en agence pour vérifier une pièce d'identité ;
- chaque service refait sa propre vérification, souvent mal ;
- les citoyens multiplient les mots de passe et les déplacements.

## 3. La solution

Vivant sépare deux moments.

| Moment | Fréquence | Ce qui est prouvé |
|---|---|---|
| **Inscription** | Une seule fois | Cette personne est bien le titulaire de cette carte, de ce visage et de ce numéro |
| **Connexion** | À chaque service | C'est bien elle qui tient son téléphone et confirme maintenant |

Le téléphone devient une **clé personnelle** : une clé cryptographique créée dans la puce sécurisée du téléphone, qui ne peut pas être copiée et ne fonctionne qu'avec le code ou l'empreinte de la personne.

Pour les organisations, Vivant est un **fournisseur d'identité** : elles ajoutent un bouton « Se connecter avec Vivant » et reçoivent une identité vérifiée, avec l'accord explicite de la personne.

## 4. Pour qui

**Les citoyens**
- Prouver qui ils sont sans se déplacer.
- Une seule identité pour tous les services.
- Voir et contrôler ce qui est partagé, et avec qui.

**Les organisations (partenaires)**

| Secteur | Usages |
|---|---|
| État | Démarches en ligne, impôts, état civil, inscriptions |
| Banques | Ouverture de compte à distance (KYC), confirmation de virement |
| Assurances | Souscription, déclaration de sinistre |
| Opérateurs | Identification à l'achat d'une puce |
| Santé | Accès au dossier médical |
| En agence | Le client scanne un QR code au guichet au lieu de montrer sa carte |

**Le marché** : le Sénégal d'abord, puis toute l'Afrique (un module par pays et par type de carte).

## 5. Comment ça marche

### 5.1 Inscription (une fois, moins de 2 minutes)

```
Téléphone            Serveur Vivant                  Sources de référence
   |  1. NIN + numéro      |                                  |
   |--------------------->|  le numéro est-il au nom du NIN ? |
   |                       |-------------------------------->| registre des SIM (opérateur)
   |  2. code SMS          |                                  |
   |<----------------------|                                  |
   |  3. code              |                                  |
   |--------------------->|                                  |
   |  4. MRZ de la carte   |  contrôles + comparaison         |
   |--------------------->|-------------------------------->| registre national (ex. DAF)
   |  5. selfie            |  vivant + visage = photo         |
   |--------------------->|                                  |
   |  6. clé publique      |  preuve de possession            |
   |   + signature         |  -> compte et appareil créés      |
   |--------------------->|                                  |
```

1. **Numéro** : il doit être enregistré au nom du titulaire chez l'opérateur (identification obligatoire des SIM).
2. **Code SMS** : prouve que la personne a la puce en main.
3. **Carte** : la MRZ (les lignes `<<<` au dos) est lue et ses chiffres de contrôle vérifiés. La carte doit être valide et correspondre au registre national.
4. **Visage** : détection du vivant, puis comparaison avec la photo de référence.
5. **Clé du téléphone** : une paire de clés ECDSA P-256 est créée dans la puce sécurisée. Le serveur ne reçoit que la clé publique, avec une signature qui prouve la possession de la clé privée.

L'identité enregistrée vient **toujours du registre national**, jamais de ce que la personne tape. Une nouvelle inscription (nouveau téléphone) révoque les anciens appareils.

### 5.2 Connexion à un service

```
Navigateur (ordinateur)        Serveur Vivant                 Téléphone
  1. « Se connecter avec Vivant »   |                              |
  ------------------------------->  |                              |
  2. numéro ou NIN                  |                              |
  ------------------------------->  |  3. demande + 3 codes         |
  4. affiche le code « 42 »         | ---------------------------> |
                                    |  5. touche « 42 »,            |
                                    |     déverrouille la clé,      |
                                    |     signe l'accord            |
                                    | <--------------------------- |
  6. retour vers le site + code     |                              |
  <-------------------------------  |                              |
  7. le site lit l'identité vérifiée (serveur à serveur)           |
```

**Le code à choisir** (étape 5) est une protection anti-arnaque : un fraudeur qui lance une connexion avec le numéro de sa victime ne peut pas lui faire accepter par erreur. Le bon code s'affiche sur l'écran du fraudeur, pas sur celui de la victime. Un mauvais choix annule la demande.

### 5.3 Deux façons pour un partenaire de s'intégrer

| | API simple | OpenID Connect |
|---|---|---|
| Pour qui | Équipes qui veulent aller vite | Équipes qui ont déjà une bibliothèque OIDC |
| Appels | `POST /v1/verifications` puis `GET /v1/verifications/{id}` | `authorize` → `token` → `id_token` signé |
| Authentification | `Authorization: Bearer sk_…` | `client_id = pk_…`, `client_secret = sk_…`, PKCE S256 |
| Résultat | JSON avec `identity` | `id_token` RS256 et `userinfo` |

Les deux reposent sur le même parcours côté utilisateur.

## 6. Principes du produit

Ces règles guident chaque décision. En cas de doute, on les relit.

1. **Simple d'abord.** Une action par écran, très peu de texte. Si ma grand-mère ne comprend pas, ce n'est pas fini.
2. **Discret.** L'accueil de l'app n'affiche ni nom complet, ni date de naissance, ni numéro national. La carte complète s'ouvre seulement avec l'empreinte.
3. **Rien sans accord.** Chaque partage est montré, donnée par donnée, et confirmé sur le téléphone.
4. **Le minimum de données.** Un partenaire n'obtient que les données qu'il a activées et justifiées.
5. **Pas de traçage entre services.** Chaque service reçoit un identifiant différent pour la même personne (`sub` « pairwise »).
6. **Pour toute l'Afrique.** Pas de drapeau, pas de référence à un seul pays dans l'interface. Les pays sont des modules.
7. **Pensé pour le terrain.** Téléphones d'entrée de gamme, réseau instable, SMS ou USSD en secours, enrôlement assisté en agence.
8. **Notre propre identité.** On s'inspire des meilleurs (itsme, Wave) sans copier leur apparence.

## 7. Architecture

```
                       ┌──────────────────────────────┐
  App mobile  ───────▶ │  vivant-server (Go)           │ ◀─────── Sites partenaires
  (Flutter, à venir)   │                               │          (API simple / OIDC)
  device-sim (test)    │  • inscription  /api/v1/…     │
                       │  • appareil     /api/v1/device│ ◀─────── Navigateur
                       │  • OIDC         /oauth/…      │          (page de connexion)
                       │  • API simple   /v1/…         │
                       │  • partenaires  /partenaires  │
                       │  • docs         /docs         │
                       └──────┬───────────┬───────────┘
                              │           │
                 registre national   SMS / opérateurs   (simulés aujourd'hui)
```

Choix techniques :
- **Go, sans dépendance externe** (bibliothèque standard uniquement). Facile à auditer, un seul binaire.
- **OpenID Connect** pour l'interopérabilité : tout le monde a une bibliothèque.
- **Pages serveur** (`html/template`) pour la connexion web, l'espace partenaires et la documentation. Pas de framework JavaScript.
- **Stockage en mémoire + fichier JSON** pour le prototype, derrière une seule structure (`store.Memory`) à remplacer par PostgreSQL.

## 8. Carte du code

```
vivant/
├── README.md
├── docs/MASTER.md              ← ce document
├── design/
│   ├── maquettes/              maquettes de l'app (fichiers .dc.html du canevas)
│   └── captures/               images de l'app et de l'espace partenaires
└── backend/
    ├── cmd/
    │   ├── vivant-server/      le serveur (point d'entrée, variables d'environnement)
    │   ├── demo-portail/       site fictif « Portail e-Services » qui intègre Vivant
    │   └── device-sim/         simulateur du téléphone (inscription, validation)
    ├── config/clients.json     services déclarés à la main (en plus des partenaires)
    └── internal/
        ├── server/             tous les points d'accès HTTP
        │   ├── server.go       routes, middleware, réponses JSON, rendu HTML
        │   ├── enroll.go       inscription en 5 étapes
        │   ├── device.go       API du téléphone (requêtes signées), approuver/refuser
        │   ├── oidc.go         OpenID Connect (authorize, token, userinfo, claims)
        │   ├── partnerapi.go   API simple /v1/verifications, lien de départ
        │   ├── partners.go     espace partenaires (inscription, tableau de bord)
        │   ├── templates/      pages HTML (connexion, partenaires, docs)
        │   └── static/         CSS et JS
        ├── store/              données (utilisateurs, appareils, demandes, partenaires)
        ├── mrz/                lecture et contrôle des MRZ (norme OACI 9303, TD1)
        ├── devicekey/          clés ECDSA du téléphone, messages signés
        ├── jwt/                signature et vérification RS256, JWKS
        ├── registry/           registre national (simulé, citoyens fictifs)
        ├── verification/       vérification faciale (simulée)
        ├── notify/             envoi de SMS (simulé)
        └── phone/              normalisation des numéros (E.164)
```

**Où ajouter quoi ?**

| Je veux… | Fichier |
|---|---|
| Ajouter une route | `internal/server/server.go`, fonction `routes()` |
| Changer une étape d'inscription | `internal/server/enroll.go` |
| Ajouter une donnée partageable (scope) | `scopeDescriptions` dans `server.go` et `userClaims` dans `oidc.go` |
| Brancher le vrai registre d'un pays | implémenter `registry.Registry` |
| Brancher un vrai prestataire de visage | implémenter `verification.FaceVerifier` |
| Brancher un vrai fournisseur SMS | implémenter `notify.SMSSender` |
| Passer à PostgreSQL | remplacer `store.Memory` en gardant ses méthodes |

## 9. Modèle de données

| Entité | Rôle | Champs clés |
|---|---|---|
| `User` | Une personne vérifiée | `Identity` (copie du registre), `Phone`, `LoA` |
| `Device` | Un téléphone activé | `PublicKey`, `RevokedAt` (un seul actif par personne) |
| `Enrollment` | Une inscription en cours (30 min max) | `Step`, tentatives, `OTPHash` |
| `AuthRequest` | Une demande de connexion ou de vérification | `Status`, `BindingCode`, `Challenge`, `Code`, `Verification` |
| `AccessToken` | Jeton OIDC (10 min) | `Hash`, `Scopes` |
| `Partner` | Une organisation inscrite | `ClientID`, `SecretHash`, `RedirectURIs`, `Scopes`, `Mode`, `Stats` |
| `AuditEvent` | Journal, visible par la personne | `Type`, `UserID`, `ClientID` |

États d'une demande : `pending_user` → `pending_device` → `approved` / `denied` / `expired`.

Ne sont **jamais** stockés en clair : codes SMS, clés secrètes des partenaires, mots de passe (PBKDF2), jetons. Seule leur empreinte est conservée.

## 10. Les API

### Inscription (app, sans authentification)

| Méthode | Chemin | Corps |
|---|---|---|
| POST | `/api/v1/enrollments` | `{nin, phone}` |
| POST | `/api/v1/enrollments/{id}/otp` | `{code}` |
| POST | `/api/v1/enrollments/{id}/document` | `{mrz}` |
| POST | `/api/v1/enrollments/{id}/selfie` | `{selfie}` (JPEG en base64) |
| POST | `/api/v1/enrollments/{id}/complete` | `{public_key, device_name, signature}` |

### Appareil (requêtes signées)

En-têtes : `X-Vivant-Device`, `X-Vivant-Timestamp`, et `X-Vivant-Signature`, qui est la signature de `METHODE\nCHEMIN\nTIMESTAMP\nsha256hex(corps)`.

| Méthode | Chemin |
|---|---|
| GET | `/api/v1/device/me` |
| GET | `/api/v1/device/requests` |
| POST | `/api/v1/device/requests/{id}/approve` : `{binding_code, signature}` |
| POST | `/api/v1/device/requests/{id}/deny` |

### Partenaires

- API simple : `POST /v1/verifications`, `GET /v1/verifications/{id}`
- OIDC : `/.well-known/openid-configuration`, `/oauth/authorize`, `/oauth/token`, `/oauth/userinfo`, `/oauth/jwks`
- La référence complète, avec exemples, se trouve sur la page `/docs` du serveur.

## 11. Sécurité

| Menace | Protection |
|---|---|
| Quelqu'un s'inscrit avec le NIN d'un autre | Numéro lié au NIN chez l'opérateur, code SMS, carte physique, visage + détection du vivant |
| Carte falsifiée | Chiffres de contrôle MRZ, comparaison au registre ; la lecture de la puce NFC est prévue |
| Vol du téléphone | Clé utilisable seulement avec code ou empreinte ; blocage de l'identité ; révocation à la réinscription |
| Fraudeur qui pousse la victime à accepter | Code à choisir parmi 3, affiché seulement sur l'écran de celui qui se connecte |
| Interception du code d'autorisation | PKCE S256 obligatoire, code à usage unique valable 60 s, demande liée au navigateur par cookie |
| Faux appareil | Chaque requête est signée par la clé du téléphone, avec horodatage |
| Partenaire curieux | Données limitées aux scopes activés, `sub` différent par service |
| Partenaire qui lit les vérifications d'un autre | Les résultats sont liés à la clé secrète du partenaire |
| Fuite d'une clé partenaire | Clé stockée en empreinte, régénération immédiate |
| Formulaire envoyé depuis un autre site | Vérification de `Sec-Fetch-Site` / `Origin` |

Avant la production :
- clé de signature dans un **HSM** ;
- limitation du débit (inscription, connexion, API) ;
- audit de sécurité externe et tests d'intrusion ;
- chiffrement des données au repos.

## 12. Ce qui est réel, ce qui est simulé

| Élément | Aujourd'hui | Production |
|---|---|---|
| Registre national | 4 citoyens fictifs | Convention avec l'État (ex. DAF au Sénégal) |
| Registre des SIM | Champ `sim_phone` du citoyen fictif | API des opérateurs |
| SMS | Écrit dans les journaux (`VIVANT_DEV=1` le renvoie dans l'API) | Agrégateur ou API opérateur |
| Visage et vivant | Accepte tout selfie | Prestataire (ex. Smile ID) ou modèle hébergé localement |
| Stockage | Mémoire + `state.json` | PostgreSQL |
| Clé de signature | Fichier PEM | HSM |
| Notification du téléphone | Le téléphone interroge le serveur | Push FCM / APNs |
| App mobile | `device-sim` en ligne de commande | Application Flutter |
| Validation des partenaires | Bouton « Demander la production » sans suite | Interface d'administration |

Identités de test :

| Personne | Téléphone | NIN | Remarque |
|---|---|---|---|
| Fatou Diop | 77 123 45 67 | 2751199000123 | |
| Moussa Ndiaye | 78 111 22 33 | 1752198500456 | |
| Aminata Coumba Sow | 70 123 45 67 | 2753200100789 | |
| Mamadou Lamine Bâ | 76 987 65 43 | 1754197800321 | Carte expirée : refusée |

## 13. Travailler sur le projet

### Installer et lancer

Prérequis : Go 1.24 ou plus récent.

```bash
cd backend
make server     # terminal 1 : http://localhost:8080
make portail    # terminal 2 : http://localhost:8081 (site de démonstration)
go run ./cmd/device-sim enroll -nin 2751199000123 -phone 771234567   # terminal 3
go run ./cmd/device-sim watch
```

Ensuite :
- Ouvrez http://localhost:8081, cliquez « Se connecter avec Vivant », saisissez `77 123 45 67`, puis tapez dans le terminal 3 le code affiché.
- Pour l'espace partenaires, ouvrez http://localhost:8080/partenaires.

### Variables d'environnement

| Variable | Défaut | Rôle |
|---|---|---|
| `VIVANT_ADDR` | `:8080` | Adresse d'écoute |
| `VIVANT_ISSUER` | `http://localhost:8080` | URL publique (doit être exacte) |
| `VIVANT_DATA_DIR` | `data-local` | Clés et données persistées |
| `VIVANT_CLIENTS_FILE` | `config/clients.json` | Services déclarés à la main |
| `VIVANT_APP_NAME` | `Vivant` | Nom affiché |
| `VIVANT_DEV` | vide | `1` = codes SMS renvoyés par l'API. **Jamais en production.** |

### Tests

```bash
cd backend && make test     # go vet + go test
```

Les tests de `internal/server` jouent des parcours complets dans un vrai serveur HTTP :
- inscription, connexion OIDC, refus, mauvais code, autre appareil, réinscription ;
- API partenaire, tableau de bord, connexion et protections.

**Toute nouvelle fonctionnalité arrive avec son test de parcours.**

### Conventions

- **Langue** : messages utilisateurs et commentaires en français ; noms de variables et de fonctions en anglais.
- **Erreurs d'API** : toujours `{"error": "code_machine", "message": "Phrase claire en français."}`.
- **Pas de dépendance externe** sans discussion : la surface d'attaque compte.
- **Données sensibles** : jamais en clair dans le stockage ni dans les journaux.
- **Pages HTML** : pas de style ni de script en ligne (la CSP les bloque). Tout passe par `static/`.
- **Format** : `gofmt` avant chaque commit.

## 14. Design

L'app suit les maquettes de `design/maquettes/` (16 écrans) ; les images sont dans `design/captures/`.

| Rôle | Couleur |
|---|---|
| Vert principal (boutons, actions) | `#34D186` |
| Vert survol | `#2BBE77` |
| Vert pour du texte sur fond blanc | `#12814B` |
| Noir (texte, fonds sombres) | `#0F1113` |
| Gris texte secondaire | `#5F6368` |
| Fond doux | `#F6F7F8` |
| Danger (refuser, supprimer) | `#DC2626` |

Règles :
- texte noir sur les boutons verts (le blanc sur ce vert n'est pas assez lisible) ;
- boutons en pilule ;
- une seule action principale par écran ;
- écrans sombres seulement pour la caméra et l'accueil.

Parcours de l'app :
- **S'inscrire** : Bienvenue → Numéro → Carte → Visage → Code.
- **Se connecter** : Accueil → Demande reçue → Confirmer (choisir le code) → C'est fait.
- **Services et profil** : Services, Activité, Profil, Sécurité, Confidentialité, Aide, Supprimer.

Écarts connus entre les maquettes et le backend :
- les maquettes lisent le NIN sur la carte, alors que l'API d'inscription le demande encore au premier écran ;
- le code à 4 chiffres est local au téléphone et n'a pas d'impact serveur.

## 15. Feuille de route

**Étape 1 — Prototype (fait)**
- Backend complet, OIDC, API simple, espace partenaires, documentation, maquettes, portail de démonstration, simulateur.

**Étape 2 — Pilote**
- [ ] Application mobile Flutter (MRZ par caméra, NFC, selfie, clé matérielle, PIN et biométrie, push).
- [ ] NIN lu sur la carte (aligner l'API sur les maquettes).
- [ ] PostgreSQL, limitation du débit, notifications push.
- [ ] Interface d'administration (validation des partenaires, clés de production, support).
- [ ] Prestataire de vérification faciale et fournisseur SMS réels.

**Étape 3 — Production au Sénégal**
- [ ] Convention avec l'État pour l'accès au registre national, et avec les opérateurs.
- [ ] Autorisation de la CDP pour les données biométriques.
- [ ] HSM, audit de sécurité, hébergement conforme.
- [ ] Premiers partenaires : un portail public, une banque, une assurance.
- [ ] Validation par USSD ou SMS pour les téléphones sans internet ; enrôlement assisté en agence.

**Étape 4 — Afrique**
- [ ] Un module par pays : registre, type de carte, indicatif.
- [ ] Langues locales et consignes vocales.
- [ ] Signature électronique, partage de documents, vérification d'âge.

## 16. Cadre légal et partenariats

Rien ne passe en production sans ces trois piliers :

1. **L'État** : accès au registre national, reconnaissance de Vivant comme moyen d'identification, intégration aux portails publics (au Sénégal : ministère du Numérique, SENUM SA, DAF).
2. **La protection des données** : déclaration et autorisation auprès de l'autorité du pays (CDP au Sénégal), en particulier pour la biométrie. Durées de conservation, droit d'accès et de suppression.
3. **Les opérateurs** : vérification qu'un numéro appartient bien au titulaire d'un NIN.

**Modèle économique envisagé** :
- gratuit pour les citoyens ;
- facturation des partenaires (par vérification ou par abonnement) ;
- tarifs spécifiques pour les services publics.

## 17. Glossaire

| Terme | Sens |
|---|---|
| **NIN** | Numéro d'identification national |
| **MRZ** | Zone de lecture automatique : les lignes `<<<` au dos de la carte (norme OACI 9303) |
| **KYC** | « Know Your Customer » : vérification de l'identité d'un client |
| **OIDC** | OpenID Connect, standard de connexion utilisé par Google, Microsoft, itsme… |
| **PKCE** | Protection du flux OIDC contre le vol du code d'autorisation |
| **Scope / donnée** | Catégorie de données demandée (`profile`, `nin`, `phone`…) |
| **`sub` pairwise** | Identifiant de la personne, différent pour chaque service |
| **Code de liaison** | Le code à 2 chiffres affiché à l'écran et choisi sur le téléphone |
| **LoA** | Niveau de garantie de l'identité (ici « substantiel ») |
| **Partenaire** | Organisation qui utilise Vivant pour vérifier ses clients |
| **`pk_test_` / `sk_test_`** | Identifiant public / clé secrète d'un partenaire en mode test |
| **CDP** | Commission de Protection des Données Personnelles (Sénégal) |
| **HSM** | Boîtier matériel qui protège les clés cryptographiques |
