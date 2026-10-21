# Vivant — application mobile

Application Flutter de Vivant : inscription (numéro, carte, visage, code secret), puis confirmation des connexions en un geste.

## Lancer

Le serveur doit tourner en mode développement (`VIVANT_DEV=1`, voir `backend/`).

```bash
flutter run --dart-define=VIVANT_API=http://10.0.2.2:8080              # émulateur Android
flutter run -d chrome --dart-define=VIVANT_API=http://localhost:8080   # navigateur
flutter test
```

Numéro de test : `77 123 45 67`. Ensuite, connectez-vous sur le portail de démonstration (http://localhost:8081) : la demande apparaît sur l'accueil.

## À faire avant un pilote

- Lecture de la MRZ par la caméra et de la puce NFC (aujourd'hui : carte de démonstration fournie par le serveur).
- Preuve de vie réelle (aujourd'hui : image fictive).
- Clé dans la puce sécurisée (Keystore / Secure Enclave) et biométrie.
- Notifications push au lieu de l'interrogation toutes les 3 secondes.

Plus de détails : `docs/MASTER.md`.
