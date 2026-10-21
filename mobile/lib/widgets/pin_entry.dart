import 'package:flutter/material.dart';

import '../app_state.dart';
import '../theme.dart';
import 'common.dart';

/// Saisie du code secret, vérifiée contre le stockage du téléphone.
class PinEntry extends StatefulWidget {
  const PinEntry({super.key, required this.onSuccess, this.keyHeight = 66});

  final VoidCallback onSuccess;
  final double keyHeight;

  @override
  State<PinEntry> createState() => _PinEntryState();
}

class _PinEntryState extends State<PinEntry> {
  var _pin = '';
  var _error = false;
  var _checking = false;

  Future<void> _type(String d) async {
    if (_checking || _pin.length == 4) return;
    setState(() {
      _pin += d;
      _error = false;
    });
    if (_pin.length < 4) return;
    setState(() => _checking = true);
    final ok = await AppScope.read(context).store.checkPin(_pin);
    if (!mounted) return;
    setState(() {
      _checking = false;
      _error = !ok;
      _pin = '';
    });
    if (ok) widget.onSuccess();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        PinDots(filled: _checking ? 4 : _pin.length, error: _error),
        SizedBox(
          height: 36,
          child: Center(
            child: Text(_error ? 'Code incorrect' : '',
                style: const TextStyle(color: VColors.danger, fontWeight: FontWeight.w600)),
          ),
        ),
        Keypad(
          onDigit: _type,
          onDelete: () => setState(() => _pin = _pin.isEmpty ? '' : _pin.substring(0, _pin.length - 1)),
          keyHeight: widget.keyHeight,
        ),
      ],
    );
  }
}

/// Demande le code secret dans une feuille ; renvoie true s'il est bon.
Future<bool> askPin(BuildContext context, {String title = 'Votre code secret'}) async {
  final ok = await showModalBottomSheet<bool>(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.white,
    showDragHandle: true,
    builder: (context) => SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 0, 12, 16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(title, style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w800)),
            const SizedBox(height: 24),
            PinEntry(keyHeight: 60, onSuccess: () => Navigator.pop(context, true)),
          ],
        ),
      ),
    ),
  );
  return ok ?? false;
}
