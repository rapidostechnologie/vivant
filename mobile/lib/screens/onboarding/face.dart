import 'dart:math';

import 'package:flutter/material.dart';

import '../../api/vivant_api.dart';
import '../../app_state.dart';
import '../../theme.dart';
import '../../widgets/common.dart';
import 'pin.dart';

/// Étape 3 : le visage (preuve de vie et comparaison avec la carte).
///
/// TODO(caméra) : capturer une courte séquence avec le package camera et
/// l'envoyer au service de preuve de vie. Le serveur actuel simule la
/// comparaison : l'image envoyée ici sert d'emplacement.
class FaceScreen extends StatefulWidget {
  const FaceScreen({super.key, required this.enrollmentId});

  final String enrollmentId;

  @override
  State<FaceScreen> createState() => _FaceScreenState();
}

class _FaceScreenState extends State<FaceScreen> with SingleTickerProviderStateMixin {
  late final _progress = AnimationController(vsync: this, duration: const Duration(milliseconds: 2200));
  var _loading = false;

  @override
  void dispose() {
    _progress.dispose();
    super.dispose();
  }

  Future<void> _capture() async {
    setState(() => _loading = true);
    final api = AppScope.read(context).api;
    try {
      await _progress.forward(from: 0);
      await api.sendSelfie(widget.enrollmentId, List.filled(1024, 0xFF));
      if (!mounted) return;
      await Navigator.of(context)
          .pushReplacement(MaterialPageRoute(builder: (_) => PinScreen(enrollmentId: widget.enrollmentId)));
    } on ApiException catch (e) {
      _progress.reset();
      if (mounted) showMessage(context, e.message);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: VColors.ink,
      body: SafeArea(
        child: Column(
          children: [
            const StepHeader(step: 3, dark: true),
            const SizedBox(height: 12),
            const Text('Votre visage',
                style: TextStyle(color: Colors.white, fontSize: 34, fontWeight: FontWeight.w800, letterSpacing: -1)),
            const SizedBox(height: 10),
            Text(_loading ? 'Ne bougez plus…' : 'Placez-le dans le cercle.',
                style: const TextStyle(color: VColors.mutedOnDark, fontSize: 17)),
            Expanded(
              child: Center(
                child: AnimatedBuilder(
                  animation: _progress,
                  builder: (_, _) => CustomPaint(
                    painter: _RingPainter(_progress.value),
                    child: const SizedBox(
                      width: 260,
                      height: 260,
                      child: Icon(Icons.face_retouching_natural_rounded, size: 120, color: VColors.dark3),
                    ),
                  ),
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
              child: PillButton(label: 'Je suis prêt', loading: _loading, onPressed: _capture),
            ),
          ],
        ),
      ),
    );
  }
}

class _RingPainter extends CustomPainter {
  _RingPainter(this.value);

  final double value;

  @override
  void paint(Canvas canvas, Size size) {
    final rect = Offset.zero & size;
    final stroke = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 6
      ..strokeCap = StrokeCap.round;
    canvas.drawOval(rect.deflate(3), stroke..color = VColors.dark3);
    canvas.drawArc(rect.deflate(3), -pi / 2, 2 * pi * value, false, stroke..color = VColors.green);
  }

  @override
  bool shouldRepaint(_RingPainter old) => old.value != value;
}
