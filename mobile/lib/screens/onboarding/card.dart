import 'package:flutter/material.dart';

import '../../api/vivant_api.dart';
import '../../app_state.dart';
import '../../config.dart';
import '../../theme.dart';
import '../../widgets/common.dart';
import 'face.dart';

/// Étape 2 : la carte d'identité (lecture de la MRZ au dos).
///
/// TODO(caméra) : lire la MRZ avec google_mlkit_text_recognition, puis la
/// puce NFC des cartes CEDEAO. En attendant : carte de démonstration en mode
/// développement, sinon saisie des trois lignes.
class CardScreen extends StatefulWidget {
  const CardScreen({super.key, required this.enrollmentId});

  final String enrollmentId;

  @override
  State<CardScreen> createState() => _CardScreenState();
}

class _CardScreenState extends State<CardScreen> with SingleTickerProviderStateMixin {
  late final _scan = AnimationController(vsync: this, duration: const Duration(milliseconds: 1600))..repeat(reverse: true);
  final _mrz = TextEditingController();
  var _loading = false;
  var _done = false;

  @override
  void dispose() {
    _scan.dispose();
    _mrz.dispose();
    super.dispose();
  }

  Future<void> _read() async {
    final api = AppScope.read(context).api;
    setState(() => _loading = true);
    try {
      final mrz = devMode && _mrz.text.trim().isEmpty ? await api.demoCard(widget.enrollmentId) : _mrz.text;
      await Future<void>.delayed(const Duration(milliseconds: 1200)); // le temps de « lire »
      await api.sendDocument(widget.enrollmentId, mrz);
      if (!mounted) return;
      setState(() => _done = true);
      await Future<void>.delayed(const Duration(milliseconds: 700));
      if (!mounted) return;
      await Navigator.of(context)
          .pushReplacement(MaterialPageRoute(builder: (_) => FaceScreen(enrollmentId: widget.enrollmentId)));
    } on ApiException catch (e) {
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
            const StepHeader(step: 2, dark: true),
            const SizedBox(height: 12),
            const Text("Votre carte\nd'identité",
                textAlign: TextAlign.center,
                style: TextStyle(color: Colors.white, fontSize: 34, height: 1.05, fontWeight: FontWeight.w800, letterSpacing: -1)),
            const SizedBox(height: 10),
            const Text('Le dos, avec les trois lignes.', style: TextStyle(color: VColors.mutedOnDark, fontSize: 17)),
            Expanded(child: Center(child: _frame())),
            if (!devMode)
              Padding(
                padding: const EdgeInsets.fromLTRB(24, 0, 24, 12),
                child: TextField(
                  controller: _mrz,
                  maxLines: 3,
                  style: const TextStyle(color: Colors.white, fontFamily: 'monospace', fontSize: 13),
                  decoration: InputDecoration(
                    hintText: 'IDSEN…',
                    hintStyle: const TextStyle(color: VColors.mutedOnDark),
                    filled: true,
                    fillColor: VColors.dark2,
                    border: OutlineInputBorder(borderRadius: BorderRadius.circular(16), borderSide: BorderSide.none),
                  ),
                ),
              ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
              child: PillButton(label: _done ? 'Carte lue' : 'Scanner ma carte', loading: _loading, onPressed: _done ? null : _read),
            ),
          ],
        ),
      ),
    );
  }

  Widget _frame() {
    const w = 300.0, h = 190.0;
    final color = _done ? VColors.green : Colors.white;
    return SizedBox(
      width: w,
      height: h,
      child: Stack(
        children: [
          Positioned.fill(
            child: AnimatedContainer(
              duration: const Duration(milliseconds: 300),
              decoration: BoxDecoration(
                color: _done ? VColors.green.withValues(alpha: 0.12) : VColors.dark2,
                borderRadius: BorderRadius.circular(20),
                border: Border.all(color: color.withValues(alpha: _done ? 1 : 0.25), width: 2),
              ),
              child: Padding(
                padding: const EdgeInsets.all(22),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.end,
                  children: [
                    for (var i = 0; i < 3; i++)
                      Container(
                        height: 8,
                        margin: const EdgeInsets.only(top: 9),
                        decoration: BoxDecoration(color: VColors.dark3, borderRadius: BorderRadius.circular(4)),
                      ),
                  ],
                ),
              ),
            ),
          ),
          if (_loading && !_done)
            AnimatedBuilder(
              animation: _scan,
              builder: (_, _) => Positioned(
                left: 14,
                right: 14,
                top: 14 + (h - 30) * _scan.value,
                child: Container(
                  height: 2,
                  decoration: BoxDecoration(
                    color: VColors.green,
                    boxShadow: [BoxShadow(color: VColors.green.withValues(alpha: 0.7), blurRadius: 12)],
                  ),
                ),
              ),
            ),
          if (_done)
            const Center(
              child: CircleAvatar(
                radius: 30,
                backgroundColor: VColors.green,
                child: Icon(Icons.check_rounded, color: VColors.ink, size: 36),
              ),
            ),
        ],
      ),
    );
  }
}
