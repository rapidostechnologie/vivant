import 'dart:async';

import 'package:flutter/material.dart';

import '../api/vivant_api.dart';
import '../app_state.dart';
import '../theme.dart';
import '../widgets/common.dart';
import '../widgets/pin_entry.dart';
import 'done.dart';

/// Confirmer une connexion : données partagées, code affiché sur le site,
/// puis code secret.
class ConfirmScreen extends StatefulWidget {
  const ConfirmScreen({super.key, required this.request});

  final PendingRequest request;

  @override
  State<ConfirmScreen> createState() => _ConfirmScreenState();
}

class _ConfirmScreenState extends State<ConfirmScreen> {
  late final Timer _tick;
  String? _code;
  var _busy = false;

  @override
  void initState() {
    super.initState();
    _tick = Timer.periodic(const Duration(seconds: 1), (_) {
      if (!mounted) return;
      if (_left == Duration.zero) Navigator.of(context).maybePop();
      setState(() {});
    });
  }

  @override
  void dispose() {
    _tick.cancel();
    super.dispose();
  }

  Duration get _left {
    final d = widget.request.expiresAt.difference(DateTime.now());
    return d.isNegative ? Duration.zero : d;
  }

  Future<void> _approve() async {
    final app = AppScope.read(context);
    final nav = Navigator.of(context);
    if (!await askPin(context, title: 'Confirmez avec votre code')) return;
    setState(() => _busy = true);
    try {
      await app.api.approve(app.identity!, widget.request, _code!);
      await nav.pushReplacement(MaterialPageRoute(builder: (_) => DoneScreen(clientName: widget.request.clientName)));
    } on ApiException catch (e) {
      if (!mounted) return;
      showMessage(context, e.code == 'binding_mismatch' ? 'Mauvais code. La demande est annulée.' : e.message);
      nav.pop();
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _deny() async {
    final app = AppScope.read(context);
    setState(() => _busy = true);
    try {
      await app.api.deny(app.identity!, widget.request);
    } on ApiException catch (_) {
      // Déjà expirée ou traitée : rien à faire.
    }
    if (mounted) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final r = widget.request;
    final left = _left;
    final mm = left.inMinutes.toString();
    final ss = (left.inSeconds % 60).toString().padLeft(2, '0');
    return Scaffold(
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(4, 0, 20, 0),
              child: Row(
                children: [
                  IconButton(
                    tooltip: 'Fermer',
                    onPressed: () => Navigator.of(context).pop(),
                    icon: const Icon(Icons.close_rounded, size: 26),
                  ),
                  const Spacer(),
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                    decoration: BoxDecoration(color: VColors.soft, borderRadius: BorderRadius.circular(20)),
                    child: Text('$mm:$ss',
                        style: const TextStyle(fontWeight: FontWeight.w700, fontFeatures: [FontFeature.tabularFigures()])),
                  ),
                ],
              ),
            ),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(24, 8, 24, 16),
                children: [
                  Text(r.clientName, style: VText.title),
                  const SizedBox(height: 8),
                  const Text('veut vérifier que c\'est bien vous.', style: VText.subtitle),
                  const SizedBox(height: 28),
                  const Text('PARTAGÉ', style: VText.section),
                  const SizedBox(height: 10),
                  Container(
                    decoration: BoxDecoration(color: VColors.background, borderRadius: BorderRadius.circular(20)),
                    padding: const EdgeInsets.symmetric(vertical: 6),
                    child: Column(
                      children: [
                        for (final c in r.claims)
                          ListTile(
                            dense: true,
                            leading: const Icon(Icons.check_circle_rounded, color: VColors.green),
                            title: Text(c, style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w600)),
                          ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 28),
                  const Text('CODE AFFICHÉ SUR LE SITE', style: VText.section),
                  const SizedBox(height: 12),
                  Row(
                    children: [
                      for (final c in r.bindingChoices)
                        Expanded(
                          child: Padding(
                            padding: EdgeInsets.only(right: c == r.bindingChoices.last ? 0 : 10),
                            child: _CodeChoice(code: c, selected: _code == c, onTap: () => setState(() => _code = c)),
                          ),
                        ),
                    ],
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
              child: Row(
                children: [
                  Expanded(
                    child: SizedBox(
                      height: 60,
                      child: OutlinedButton(
                        onPressed: _busy ? null : _deny,
                        style: OutlinedButton.styleFrom(
                          foregroundColor: VColors.ink,
                          side: const BorderSide(color: VColors.line, width: 1.5),
                          shape: const StadiumBorder(),
                          textStyle: const TextStyle(fontSize: 17, fontWeight: FontWeight.w700),
                        ),
                        child: const Text('Refuser'),
                      ),
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    flex: 2,
                    child: PillButton(label: 'Confirmer', loading: _busy, onPressed: _code == null ? null : _approve),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _CodeChoice extends StatelessWidget {
  const _CodeChoice({required this.code, required this.selected, required this.onTap});

  final String code;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: true,
      selected: selected,
      label: 'Code $code',
      excludeSemantics: true,
      child: GestureDetector(
        onTap: onTap,
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 150),
          height: 76,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: selected ? VColors.green : VColors.soft,
            borderRadius: BorderRadius.circular(20),
          ),
          child: Text(code, style: const TextStyle(fontSize: 30, fontWeight: FontWeight.w800, letterSpacing: 2)),
        ),
      ),
    );
  }
}
