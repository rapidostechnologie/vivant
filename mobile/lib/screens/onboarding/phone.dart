import 'package:flutter/material.dart';

import '../../api/vivant_api.dart';
import '../../app_state.dart';
import '../../config.dart';
import '../../theme.dart';
import '../../widgets/common.dart';
import 'card.dart';

/// Indicatifs proposés (la liste s'allonge pays par pays).
const _countries = [
  ('Sénégal', '221', 9),
  ("Côte d'Ivoire", '225', 10),
  ('Mali', '223', 8),
  ('Burkina Faso', '226', 8),
  ('Guinée', '224', 9),
  ('Niger', '227', 8),
  ('Bénin', '229', 10),
  ('Togo', '228', 8),
  ('Cameroun', '237', 9),
  ('Gabon', '241', 8),
  ('Maroc', '212', 9),
  ('Nigeria', '234', 10),
  ('Ghana', '233', 9),
  ('Kenya', '254', 9),
];

/// Étape 1 : numéro de téléphone, puis code SMS.
class PhoneScreen extends StatefulWidget {
  const PhoneScreen({super.key});

  @override
  State<PhoneScreen> createState() => _PhoneScreenState();
}

class _PhoneScreenState extends State<PhoneScreen> {
  var _country = _countries.first;
  var _digits = '';
  var _loading = false;

  // Après l'envoi du SMS.
  String? _enrollmentId;
  var _code = '';

  bool get _codeStep => _enrollmentId != null;
  int get _max => _codeStep ? 6 : _country.$3;

  void _type(String d) {
    if (_loading) return;
    setState(() {
      if (_codeStep) {
        if (_code.length < 6) _code += d;
      } else if (_digits.length < _max) {
        _digits += d;
      }
    });
    if (_codeStep && _code.length == 6) _verify();
  }

  void _delete() => setState(() {
        if (_codeStep) {
          if (_code.isNotEmpty) _code = _code.substring(0, _code.length - 1);
        } else if (_digits.isNotEmpty) {
          _digits = _digits.substring(0, _digits.length - 1);
        }
      });

  Future<void> _send() async {
    setState(() => _loading = true);
    try {
      final res = await AppScope.read(context).api.startEnrollment('+${_country.$2}$_digits');
      setState(() {
        _enrollmentId = res['enrollment_id'] as String;
        _code = '';
      });
      final otp = res['dev_otp'] as String?;
      if (devMode && otp != null) {
        // Mode développement : le serveur donne le code, on le « reçoit ».
        for (final d in otp.split('')) {
          await Future<void>.delayed(const Duration(milliseconds: 90));
          if (!mounted) return;
          setState(() => _code += d);
        }
        await _verify();
      }
    } on ApiException catch (e) {
      if (mounted) showMessage(context, e.code == 'holder_not_found' ? 'Ce numéro n\'est pas encore pris en charge.' : e.message);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _verify() async {
    final id = _enrollmentId!;
    setState(() => _loading = true);
    try {
      await AppScope.read(context).api.verifyOtp(id, _code);
      if (!mounted) return;
      await Navigator.of(context).push(MaterialPageRoute(builder: (_) => CardScreen(enrollmentId: id)));
    } on ApiException catch (e) {
      if (!mounted) return;
      showMessage(context, e.message);
      setState(() => _code = '');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _pickCountry() async {
    final picked = await showModalBottomSheet<(String, String, int)>(
      context: context,
      showDragHandle: true,
      backgroundColor: Colors.white,
      builder: (context) => ListView(
        shrinkWrap: true,
        children: [
          for (final c in _countries)
            ListTile(
              title: Text(c.$1, style: const TextStyle(fontWeight: FontWeight.w600)),
              trailing: Text('+${c.$2}', style: const TextStyle(color: VColors.muted)),
              onTap: () => Navigator.pop(context, c),
            ),
        ],
      ),
    );
    if (picked != null) {
      setState(() {
        _country = picked;
        _digits = '';
      });
    }
  }

  String get _formatted {
    final b = StringBuffer();
    for (var i = 0; i < _digits.length; i++) {
      if (i == 2 || (i >= 5 && (i - 5).isEven)) b.write(' '); // 77 123 45 67
      b.write(_digits[i]);
    }
    return b.toString();
  }

  @override
  Widget build(BuildContext context) {
    return PopScope(
      canPop: !_codeStep,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) setState(() => _enrollmentId = null);
      },
      child: Scaffold(
        body: SafeArea(
          child: Column(
            children: [
              const StepHeader(step: 1),
              Expanded(
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 24),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const SizedBox(height: 12),
                      Text(_codeStep ? 'Code reçu\npar SMS' : 'Votre\nnuméro', style: VText.title),
                      const SizedBox(height: 10),
                      Text(
                        _codeStep ? 'Envoyé au +${_country.$2} $_formatted' : 'Celui de votre puce, à votre nom.',
                        style: VText.subtitle,
                      ),
                      const SizedBox(height: 36),
                      if (_codeStep) _codeBoxes() else _phoneField(),
                      const Spacer(),
                    ],
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 12),
                child: Keypad(onDigit: _type, onDelete: _delete, keyHeight: 62),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(24, 8, 24, 16),
                child: _codeStep
                    ? TextButton(
                        onPressed: _loading ? null : _send,
                        style: TextButton.styleFrom(minimumSize: const Size.fromHeight(60), foregroundColor: VColors.ink),
                        child: _loading
                            ? const SizedBox(width: 22, height: 22, child: CircularProgressIndicator(strokeWidth: 2.6))
                            : const Text('Renvoyer le code', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
                      )
                    : PillButton(
                        label: 'Recevoir le code',
                        loading: _loading,
                        onPressed: _digits.length == _max ? _send : null,
                      ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _phoneField() {
    return Row(
      children: [
        Material(
          color: VColors.soft,
          shape: const StadiumBorder(),
          child: InkWell(
            customBorder: const StadiumBorder(),
            onTap: _pickCountry,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
              child: Row(children: [
                Text('+${_country.$2}', style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w700)),
                const Icon(Icons.expand_more_rounded, size: 22),
              ]),
            ),
          ),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Text(
            _digits.isEmpty ? '77 000 00 00' : _formatted,
            style: TextStyle(
              fontSize: 28,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.5,
              color: _digits.isEmpty ? VColors.line : VColors.ink,
            ),
          ),
        ),
      ],
    );
  }

  Widget _codeBoxes() {
    return Row(
      children: [
        for (var i = 0; i < 6; i++)
          Expanded(
            child: Container(
              height: 62,
              margin: EdgeInsets.only(right: i < 5 ? 8 : 0),
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: VColors.soft,
                borderRadius: BorderRadius.circular(14),
                border: Border.all(color: i == _code.length ? VColors.green : Colors.transparent, width: 2),
              ),
              child: Text(i < _code.length ? _code[i] : '',
                  style: const TextStyle(fontSize: 26, fontWeight: FontWeight.w700)),
            ),
          ),
      ],
    );
  }
}
