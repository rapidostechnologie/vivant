import 'dart:convert';
import 'dart:math';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:pointycastle/export.dart';

import '../crypto/device_key.dart';

/// Ce que le téléphone garde après l'inscription.
class DeviceIdentity {
  const DeviceIdentity({required this.deviceId, required this.key, required this.firstName});

  final String deviceId;
  final DeviceKey key;
  final String firstName;
}

/// Stockage sécurisé du téléphone (Keychain / Keystore).
class DeviceStore {
  DeviceStore([FlutterSecureStorage? storage]) : _storage = storage ?? const FlutterSecureStorage();

  final FlutterSecureStorage _storage;

  Future<DeviceIdentity?> load() async {
    final values = await _storage.readAll();
    final id = values['device_id'];
    final key = values['private_key'];
    if (id == null || key == null) return null;
    return DeviceIdentity(
      deviceId: id,
      key: DeviceKey.fromPrivateHex(key),
      firstName: values['first_name'] ?? '',
    );
  }

  Future<void> save(DeviceIdentity identity, String pin) async {
    await _storage.write(key: 'device_id', value: identity.deviceId);
    await _storage.write(key: 'private_key', value: identity.key.privateHex);
    await _storage.write(key: 'first_name', value: identity.firstName);
    await setPin(pin);
  }

  Future<void> setPin(String pin) async {
    final salt = base64Url.encode(List.generate(16, (_) => Random.secure().nextInt(256)));
    await _storage.write(key: 'pin_salt', value: salt);
    await _storage.write(key: 'pin_hash', value: _hash(salt, pin));
  }

  Future<bool> checkPin(String pin) async {
    final salt = await _storage.read(key: 'pin_salt');
    final hash = await _storage.read(key: 'pin_hash');
    return salt != null && hash != null && _hash(salt, pin) == hash;
  }

  Future<void> clear() => _storage.deleteAll();

  static String _hash(String salt, String pin) {
    final derivator = PBKDF2KeyDerivator(HMac(SHA256Digest(), 64))
      ..init(Pbkdf2Parameters(utf8.encode(salt), 50000, 32));
    return base64Url.encode(derivator.process(utf8.encode(pin)));
  }
}
