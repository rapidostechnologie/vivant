# Vivant : backend

Vivant est une identité numérique vérifiée, sur le téléphone. Elle garantit qu'une personne qui se connecte à un service en ligne (démarches administratives, banque…) est bien la vraie personne.

Ce dépôt contient le **backend en Go**, sans dépendance externe :

| Commande | Rôle |
|---|---|
| `cmd/vivant-server` | Fournisseur d'identité : inscription vérifiée, API du téléphone, OpenID Connect |
| `cmd/demo-portail` | Portail administratif **fictif** qui intègre « Se connecter avec Vivant » |
| `cmd/device-sim` | Simulateur du téléphone, en attendant l'application mobile |

## Démarrage rapide

Prérequis : Go 1.24 ou plus récent. Ouvrez trois terminaux dans `backend/` :

```bash
# Terminal 1 : serveur Vivant (mode développement : les codes SMS s'affichent)
make server

# Terminal 2 : portail de démonstration sur http://localhost:8081
make portail

# Terminal 3 : inscrire un téléphone simulé, puis attendre les demandes
go run ./cmd/device-sim enroll -nin 2751199000123 -phone 771234567
go run ./cmd/device-sim watch
```

Ouvrez ensuite http://localhost:8081, cliquez sur **Se connecter avec Vivant** et saisissez `77 123 45 67`. Un code à 2 chiffres s'affiche dans le navigateur ; tapez-le dans le terminal 3. Vous arrivez dans votre espace avec votre identité vérifiée.

Tests : `make test`.

### Citoyens fictifs du registre simulé

| NIN | Nom | Téléphone | Particularité |
|---|---|---|---|
| 2751199000123 | Fatou Diop | 77 123 45 67 | |
| 1752198500456 | Moussa Ndiaye | 78 111 22 33 | |
| 2753200100789 | Aminata Coumba Sow | 70 123 45 67 | |
| 1754197800321 | Mamadou Lamine Bâ | 76 987 65 43 | Carte expirée : inscription refusée |

## Comment Vivant prouve que c'est la vraie personne

### 1. Inscription (une seule fois)

1. **NIN et numéro de téléphone** : le numéro doit être enregistré au nom du titulaire chez l'opérateur (identification obligatoire des SIM). Un code est envoyé par SMS.
2. **Code SMS** : prouve que la personne a la puce en main.
3. **Carte d'identité CEDEAO** : la MRZ (les 3 lignes au verso) est lue et ses chiffres de contrôle vérifiés. La carte doit être valide et correspondre au registre national pour ce NIN.
4. **Selfie avec détection du vivant** : il est comparé à la photo de référence.
5. **Clé du téléphone** : une clé ECDSA P-256 est créée dans la puce sécurisée du téléphone et protégée par PIN ou biométrie. Le serveur ne reçoit que la clé publique, avec une signature qui prouve la possession de la clé privée.

L'identité enregistrée vient **du registre national**, jamais de ce que l'utilisateur saisit. Une nouvelle inscription (changement de téléphone) révoque les anciens appareils.

### 2. Connexion à un service

1. Le service redirige vers Vivant (OpenID Connect, flux *authorization code* + PKCE).
2. L'utilisateur saisit son numéro ou son NIN. Le navigateur affiche un **code à 2 chiffres**.
3. Le téléphone reçoit la demande et affiche le service et les données demandées, avec 3 codes au choix. L'utilisateur choisit celui de l'écran, déverrouille sa clé et **signe** son accord.
4. Le service reçoit un `id_token` signé avec l'identité vérifiée.

Un fraudeur qui connaît le numéro de sa victime ne peut rien faire sans **le téléphone** (clé matérielle), **le PIN ou l'empreinte** de la victime, et le **bon code** affiché sur son propre écran. Un mauvais choix annule la demande.

### Protections en place

- PKCE obligatoire, codes d'autorisation à usage unique valables 60 s.
- Demande de connexion liée au navigateur par un cookie : un tiers ne peut pas récupérer le code.
- Requêtes du téléphone signées (méthode, chemin, horodatage, empreinte du corps).
- Identifiant différent pour chaque service (*pairwise*) : les services ne peuvent pas croiser leurs données.
- Seules les données demandées et acceptées sont transmises.
- La page de connexion ne révèle pas si un numéro possède un compte.
- Le journal d'audit est visible par l'utilisateur dans l'historique.

## Espace partenaires et API simple

Les banques, assurances et administrations s'inscrivent elles-mêmes :

| Page | Adresse |
|---|---|
| Présentation | http://localhost:8080/partenaires |
| Inscription (clés de test immédiates) | http://localhost:8080/partenaires/inscription |
| Tableau de bord (clés, adresses de retour, données, statistiques) | http://localhost:8080/partenaires/tableau |
| Documentation avec code Node.js, PHP et Python | http://localhost:8080/docs |

L'API simple tient en deux appels, avec la clé secrète `sk_test_…` :

```bash
# 1. Créer une vérification, puis rediriger le client vers "url"
curl http://localhost:8080/v1/verifications \
  -H "Authorization: Bearer $VIVANT_SECRET_KEY" \
  -H "Content-Type: application/json" \
  -d '{"redirect_url": "http://localhost:3000/retour", "data": ["profile"]}'

# 2. Le client revient sur ?verification_id=…, lire le résultat
curl http://localhost:8080/v1/verifications/ver_… \
  -H "Authorization: Bearer $VIVANT_SECRET_KEY"
```

Les partenaires qui préfèrent le standard utilisent OpenID Connect avec `pk_test_…` comme `client_id` et la même clé secrète comme `client_secret`. La mise en production (« Demander la production ») est validée par l'équipe Vivant ; l'interface d'administration reste à construire.

## Intégrer un service (exemple : un portail de démarches)

Vivant est un fournisseur **OpenID Connect standard**. Toute bibliothèque OIDC fonctionne (Spring Security, Laravel Socialite, `openid-client` en Node, `go-oidc`…).

- Découverte : `GET /.well-known/openid-configuration`
- Scopes : `openid` (obligatoire), `profile`, `birth` (lieu de naissance), `nin`, `phone`, `address`
- Niveau de garantie : claim `acr` = `urn:vivant:loa:substantial`
- Méthode : claim `amr` = `["hwk", "user"]` (clé matérielle + PIN ou biométrie)

Les services sont déclarés dans `config/clients.json`.

## API du téléphone (pour l'application mobile)

Inscription, sans authentification :

| Méthode | Chemin | Corps |
|---|---|---|
| POST | `/api/v1/enrollments` | `{nin, phone}` |
| POST | `/api/v1/enrollments/{id}/otp` | `{code}` |
| POST | `/api/v1/enrollments/{id}/document` | `{mrz, document_image?}` |
| POST | `/api/v1/enrollments/{id}/selfie` | `{selfie}` (JPEG en base64) |
| POST | `/api/v1/enrollments/{id}/complete` | `{public_key, device_name, signature}` |

`signature` = signature de `vivant-enroll:{id}`. La clé publique est acceptée au format SPKI DER (Android) ou en point non compressé de 65 octets (iOS), en base64.

Appareil inscrit : en-têtes `X-Vivant-Device`, `X-Vivant-Timestamp` (Unix), `X-Vivant-Signature` = signature de `METHODE\nCHEMIN\nTIMESTAMP\nsha256hex(corps)`.

| Méthode | Chemin | Corps |
|---|---|---|
| GET | `/api/v1/device/me` | identité, appareils, historique |
| GET | `/api/v1/device/requests` | demandes en attente |
| POST | `/api/v1/device/requests/{id}/approve` | `{binding_code, signature}` : signature de `vivant-approve:{id}:{challenge}:{binding_code}` |
| POST | `/api/v1/device/requests/{id}/deny` | `{}` |

Signatures : ECDSA P-256 / SHA-256, au format DER ou brut r‖s, en base64.

## Ce qui est simulé

| Élément | Prototype | Pour la production |
|---|---|---|
| Registre national | 4 citoyens fictifs (`internal/registry`) | Convention avec l'État pour accéder au fichier de la DAF |
| Registre des SIM | Champ `sim_phone` | API des opérateurs (Orange/Sonatel, Free, Expresso) |
| SMS | Écrits dans les journaux | Agrégateur ou API SMS opérateur |
| Visage et vivant | Accepte tout selfie | Prestataire (Smile ID…) ou modèle hébergé au Sénégal |
| Stockage | Mémoire + fichier JSON | PostgreSQL |
| Clé de signature | Fichier PEM | HSM |
| Notification du téléphone | Le téléphone interroge le serveur | Push FCM / APNs |

## Prochaines étapes

1. **Application mobile Flutter** : scan de la MRZ, lecture NFC de la puce, selfie, clé dans la Secure Enclave ou StrongBox, PIN et biométrie, notifications.
2. PostgreSQL, limitation du débit (inscription et connexion), notifications push.
3. Validation par **USSD ou SMS** pour les téléphones sans internet.
4. Démarches avec la **CDP** (données biométriques) et présentation à l'État et à **SENUM SA** pour l'intégration aux portails publics.
5. Multi-pays : un module de registre par pays et par type de pièce d'identité.
