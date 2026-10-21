import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:pointycastle/export.dart';

/// Clé ECDSA P-256 du téléphone.
///
/// Prototype : la clé est générée en Dart et rangée dans le stockage
/// sécurisé. En production, elle doit être créée et utilisée dans la puce
/// sécurisée (Android Keystore/StrongBox, Secure Enclave) avec
/// déverrouillage biométrique, sans jamais quitter la puce.
class DeviceKey {
  DeviceKey._(this._private, this._public);

  final ECPrivateKey _private;
  final ECPublicKey _public;

  static final _domain = ECDomainParameters('secp256r1');

  factory DeviceKey.generate() {
    final seed = Uint8List.fromList(List.generate(32, (_) => Random.secure().nextInt(256)));
    final random = FortunaRandom()..seed(KeyParameter(seed));
    final generator = ECKeyGenerator()
      ..init(ParametersWithRandom(ECKeyGeneratorParameters(_domain), random));
    final pair = generator.generateKeyPair();
    return DeviceKey._(pair.privateKey, pair.publicKey);
  }

  factory DeviceKey.fromPrivateHex(String hex) {
    final d = BigInt.parse(hex, radix: 16);
    return DeviceKey._(ECPrivateKey(d, _domain), ECPublicKey(_domain.G * d, _domain));
  }

  String get privateHex => _private.d!.toRadixString(16);

  /// Clé publique : point non compressé de 65 octets, en base64url.
  String get publicKeyBase64 => _b64(_public.Q!.getEncoded(false));

  /// Signature ECDSA-SHA256 déterministe (RFC 6979), format brut r‖s.
  String sign(List<int> message) {
    final signer = ECDSASigner(SHA256Digest(), HMac(SHA256Digest(), 64))
      ..init(true, PrivateKeyParameter<ECPrivateKey>(_private));
    final sig = signer.generateSignature(Uint8List.fromList(message)) as ECSignature;
    return _b64(Uint8List.fromList([..._fixed32(sig.r), ..._fixed32(sig.s)]));
  }

  bool verify(List<int> message, String signature) {
    final raw = base64Url.decode(base64Url.normalize(signature));
    final sig = ECSignature(_toBigInt(raw.sublist(0, 32)), _toBigInt(raw.sublist(32)));
    final verifier = ECDSASigner(SHA256Digest())..init(false, PublicKeyParameter<ECPublicKey>(_public));
    return verifier.verifySignature(Uint8List.fromList(message), sig);
  }

  static String _b64(List<int> bytes) => base64Url.encode(bytes).replaceAll('=', '');

  static List<int> _fixed32(BigInt v) {
    final hex = v.toRadixString(16).padLeft(64, '0');
    return [for (var i = 0; i < 64; i += 2) int.parse(hex.substring(i, i + 2), radix: 16)];
  }

  static BigInt _toBigInt(List<int> bytes) =>
      bytes.fold(BigInt.zero, (acc, b) => (acc << 8) | BigInt.from(b));
}

/// Messages signés, identiques à ceux du serveur (internal/devicekey).
abstract final class SignedMessages {
  static List<int> enrollment(String enrollmentId) => utf8.encode('vivant-enroll:$enrollmentId');

  static List<int> approval(String requestId, String challenge, String bindingCode) =>
      utf8.encode('vivant-approve:$requestId:$challenge:$bindingCode');

  static List<int> request(String method, String path, String timestamp, List<int> body) {
    final hash = SHA256Digest().process(Uint8List.fromList(body));
    final hex = hash.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
    return utf8.encode('$method\n$path\n$timestamp\n$hex');
  }
}
