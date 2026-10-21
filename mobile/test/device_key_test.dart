import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:vivant/crypto/device_key.dart';

void main() {
  test('la signature se vérifie et ne vaut que pour son message', () {
    final key = DeviceKey.generate();
    final msg = SignedMessages.approval('req_1', 'chal', '42');
    final sig = key.sign(msg);
    expect(base64Url.decode(base64Url.normalize(sig)), hasLength(64));
    expect(key.verify(msg, sig), isTrue);
    expect(key.verify(SignedMessages.approval('req_1', 'chal', '43'), sig), isFalse);
  });

  test('la clé se recharge depuis le stockage', () {
    final key = DeviceKey.generate();
    final again = DeviceKey.fromPrivateHex(key.privateHex);
    expect(again.publicKeyBase64, key.publicKeyBase64);
    expect(base64Url.decode(base64Url.normalize(key.publicKeyBase64)), hasLength(65));
    final msg = SignedMessages.enrollment('enr_1');
    expect(key.verify(msg, again.sign(msg)), isTrue);
  });

  test('message de requête identique au serveur', () {
    final msg = utf8.decode(SignedMessages.request('GET', '/api/v1/device/me', '1700000000', const []));
    expect(msg,
        'GET\n/api/v1/device/me\n1700000000\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855');
  });
}
