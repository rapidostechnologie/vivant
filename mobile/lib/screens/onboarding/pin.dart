import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

import '../../api/vivant_api.dart';
import '../../app_state.dart';
import '../../crypto/device_key.dart';
import '../../storage/device_store.dart';
import '../../theme.dart';
import '../../widgets/common.dart';

/// Étape 4 : code secret, puis activation du téléphone (clé de l'appareil).
class PinScreen extends StatefulWidget {
  const PinScreen({super.key, required this.enrollmentId});

  final String enrollmentId;

  @override
  State<PinScreen> createState() => _PinScreenState();
}

class _PinScreenState extends State<PinScreen> {
  var _pin = '';
  String? _first;
  var _error = false;
  var _loading = false;

  void _type(String d) {
    if (_loading || _pin.length == 4) return;
    setState(() {
      _pin += d;
      _error = false;
    });
    if (_pin.length == 4) Future.delayed(const Duration(milliseconds: 150), _next);
  }

  Future<void> _next() async {
    if (_first == null) {
      setState(() {
        _first = _pin;
        _pin = '';
      });
      return;
    }
    if (_first != _pin) {
      setState(() {
        _error = true;
        _pin = '';
        _first = null;
      });
      if (mounted) showMessage(context, 'Les codes ne correspondent pas.');
      return;
    }
    await _activate();
  }

  Future<void> _activate() async {
    final app = AppScope.read(context);
    setState(() => _loading = true);
    try {
      final key = DeviceKey.generate();
      final res = await app.api.completeEnrollment(widget.enrollmentId, key, _deviceName());
      final identity = res['identity'] as Map<String, dynamic>;
      final given = (identity['given_names'] as String? ?? '').split(' ').first;
      await app.enrolled(
        DeviceIdentity(deviceId: res['device_id'] as String, key: key, firstName: _capitalize(given)),
        _pin,
      );
      if (mounted) Navigator.of(context).popUntil((r) => r.isFirst);
    } on ApiException catch (e) {
      if (!mounted) return;
      showMessage(context, e.message);
      setState(() {
        _pin = '';
        _first = null;
      });
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  static String _deviceName() => switch (defaultTargetPlatform) {
        TargetPlatform.iOS => 'iPhone',
        TargetPlatform.android => 'Android',
        _ => kIsWeb ? 'Navigateur' : 'Téléphone',
      };

  static String _capitalize(String s) => s.isEmpty ? s : s[0].toUpperCase() + s.substring(1).toLowerCase();

  @override
  Widget build(BuildContext context) {
    final confirming = _first != null;
    return Scaffold(
      body: SafeArea(
        child: Column(
          children: [
            const StepHeader(step: 4),
            const SizedBox(height: 12),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 24),
              child: Column(
                children: [
                  Text(confirming ? 'Confirmez\nle code' : 'Votre code\nsecret',
                      textAlign: TextAlign.center, style: VText.title),
                  const SizedBox(height: 10),
                  Text(confirming ? 'Une seconde fois.' : '4 chiffres, rien que pour vous.', style: VText.subtitle),
                ],
              ),
            ),
            const Spacer(),
            _loading
                ? const SizedBox(height: 22, width: 22, child: CircularProgressIndicator(strokeWidth: 2.6))
                : PinDots(filled: _pin.length, error: _error),
            const Spacer(),
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 0, 12, 24),
              child: Keypad(
                onDigit: _type,
                onDelete: () => setState(() => _pin = _pin.isEmpty ? '' : _pin.substring(0, _pin.length - 1)),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
