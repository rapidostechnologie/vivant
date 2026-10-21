/// Adresse du serveur Vivant.
///
/// Émulateur Android : `--dart-define=VIVANT_API=http://10.0.2.2:8080`.
const apiBaseUrl = String.fromEnvironment('VIVANT_API', defaultValue: 'http://localhost:8080');

/// Mode développement : code SMS et carte de démonstration fournis par le
/// serveur (VIVANT_DEV=1), à la place du SMS et de la caméra.
const devMode = bool.fromEnvironment('VIVANT_DEV', defaultValue: true);
