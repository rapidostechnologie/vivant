import 'dart:convert';

import 'package:http/http.dart' as http;

import '../config.dart';
import '../crypto/device_key.dart';
import '../storage/device_store.dart';

/// Erreur renvoyée par le serveur : `{"error": "...", "message": "..."}`.
class ApiException implements Exception {
  const ApiException(this.status, this.code, this.message);

  final int status;
  final String code;
  final String message;

  @override
  String toString() => message;
}

/// Une demande de connexion à valider sur le téléphone.
class PendingRequest {
  PendingRequest.fromJson(Map<String, dynamic> j)
      : id = j['id'] as String,
        clientName = j['client_name'] as String,
        claims = List<String>.from(j['claims'] as List),
        challenge = j['challenge'] as String,
        bindingChoices = List<String>.from(j['binding_choices'] as List),
        createdAt = DateTime.parse(j['created_at'] as String).toLocal(),
        expiresAt = DateTime.parse(j['expires_at'] as String).toLocal();

  final String id;
  final String clientName;
  final List<String> claims;
  final String challenge;
  final List<String> bindingChoices;
  final DateTime createdAt;
  final DateTime expiresAt;
}

class VivantApi {
  VivantApi({http.Client? client, this.baseUrl = apiBaseUrl}) : _client = client ?? http.Client();

  final http.Client _client;
  final String baseUrl;

  // --- Inscription ---

  Future<Map<String, dynamic>> startEnrollment(String phone) async =>
      await _send('POST', '/api/v1/enrollments', body: {'phone': phone}) as Map<String, dynamic>;

  Future<Map<String, dynamic>> verifyOtp(String id, String code) async =>
      await _send('POST', '/api/v1/enrollments/$id/otp', body: {'code': code}) as Map<String, dynamic>;

  /// Carte de démonstration (serveur en mode développement uniquement).
  Future<String> demoCard(String id) async =>
      (await _send('GET', '/api/v1/dev/enrollments/$id/card'))['mrz'] as String;

  Future<Map<String, dynamic>> sendDocument(String id, String mrz) async =>
      await _send('POST', '/api/v1/enrollments/$id/document', body: {'mrz': mrz}) as Map<String, dynamic>;

  Future<Map<String, dynamic>> sendSelfie(String id, List<int> jpeg) async =>
      await _send('POST', '/api/v1/enrollments/$id/selfie', body: {'selfie': base64.encode(jpeg)}) as Map<String, dynamic>;

  Future<Map<String, dynamic>> completeEnrollment(String id, DeviceKey key, String deviceName) async =>
      await _send('POST', '/api/v1/enrollments/$id/complete', body: {
        'public_key': key.publicKeyBase64,
        'device_name': deviceName,
        'signature': key.sign(SignedMessages.enrollment(id)),
      }) as Map<String, dynamic>;

  Future<List<Map<String, dynamic>>> services() async =>
      List<Map<String, dynamic>>.from(await _send('GET', '/api/v1/services') as List);

  // --- Téléphone inscrit (requêtes signées) ---

  Future<Map<String, dynamic>> me(DeviceIdentity d) async =>
      await _signed(d, 'GET', '/api/v1/device/me') as Map<String, dynamic>;

  Future<List<PendingRequest>> pendingRequests(DeviceIdentity d) async {
    final list = await _signed(d, 'GET', '/api/v1/device/requests') as List;
    return [for (final j in list) PendingRequest.fromJson(j as Map<String, dynamic>)];
  }

  Future<void> approve(DeviceIdentity d, PendingRequest r, String code) => _signed(
        d,
        'POST',
        '/api/v1/device/requests/${r.id}/approve',
        {'binding_code': code, 'signature': d.key.sign(SignedMessages.approval(r.id, r.challenge, code))},
      );

  Future<void> deny(DeviceIdentity d, PendingRequest r) =>
      _signed(d, 'POST', '/api/v1/device/requests/${r.id}/deny', {});

  Future<void> revoke(DeviceIdentity d) => _signed(d, 'POST', '/api/v1/device/revoke', {});

  // --- Transport ---

  Future<dynamic> _signed(DeviceIdentity d, String method, String path, [Map<String, dynamic>? body]) {
    final raw = body == null ? <int>[] : utf8.encode(jsonEncode(body));
    final ts = (DateTime.now().millisecondsSinceEpoch ~/ 1000).toString();
    return _send(method, path, rawBody: raw, headers: {
      'X-Vivant-Device': d.deviceId,
      'X-Vivant-Timestamp': ts,
      'X-Vivant-Signature': d.key.sign(SignedMessages.request(method, path, ts, raw)),
    });
  }

  Future<dynamic> _send(String method, String path,
      {Map<String, dynamic>? body, List<int>? rawBody, Map<String, String>? headers}) async {
    final request = http.Request(method, Uri.parse('$baseUrl$path'))..headers.addAll(headers ?? {});
    final bytes = rawBody ?? (body == null ? null : utf8.encode(jsonEncode(body)));
    if (bytes != null && bytes.isNotEmpty) {
      request.headers['Content-Type'] = 'application/json';
      request.bodyBytes = bytes;
    }
    final http.Response response;
    try {
      response = await http.Response.fromStream(await _client.send(request).timeout(const Duration(seconds: 20)));
    } catch (_) {
      throw const ApiException(0, 'network', 'Pas de connexion. Vérifiez votre réseau et réessayez.');
    }
    final decoded = response.body.isEmpty ? null : jsonDecode(utf8.decode(response.bodyBytes));
    if (response.statusCode >= 300) {
      final map = decoded is Map ? decoded : const {};
      throw ApiException(
        response.statusCode,
        map['error']?.toString() ?? 'http_${response.statusCode}',
        map['message']?.toString() ?? 'Une erreur est survenue. Réessayez.',
      );
    }
    return decoded;
  }
}
