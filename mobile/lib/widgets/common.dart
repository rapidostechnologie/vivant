import 'package:flutter/material.dart';

import '../theme.dart';

/// Bouton principal en pilule. [dark] : noir au lieu de vert.
class PillButton extends StatelessWidget {
  const PillButton({super.key, required this.label, this.onPressed, this.dark = false, this.loading = false, this.danger = false});

  final String label;
  final VoidCallback? onPressed;
  final bool dark;
  final bool loading;
  final bool danger;

  @override
  Widget build(BuildContext context) {
    final bg = danger ? VColors.danger : (dark ? VColors.ink : VColors.green);
    final fg = (dark || danger) ? Colors.white : VColors.ink;
    return SizedBox(
      width: double.infinity,
      height: 60,
      child: FilledButton(
        onPressed: loading ? null : onPressed,
        style: FilledButton.styleFrom(
          backgroundColor: bg,
          foregroundColor: fg,
          disabledBackgroundColor: bg.withValues(alpha: 0.45),
          disabledForegroundColor: fg.withValues(alpha: 0.8),
          shape: const StadiumBorder(),
          textStyle: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
        ),
        child: loading
            ? SizedBox(width: 22, height: 22, child: CircularProgressIndicator(strokeWidth: 2.6, color: fg))
            : Text(label),
      ),
    );
  }
}

/// Logo : une personne qui lève les bras (un point et un V).
class VivantLogo extends StatelessWidget {
  const VivantLogo({super.key, this.size = 108});

  final double size;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        color: VColors.green,
        borderRadius: BorderRadius.circular(size * 0.32),
        boxShadow: [BoxShadow(color: VColors.green.withValues(alpha: 0.35), blurRadius: 50, offset: const Offset(0, 20))],
      ),
      child: CustomPaint(painter: _LogoPainter()),
    );
  }
}

class _LogoPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final s = size.width / 48;
    final paint = Paint()..color = VColors.ink;
    canvas.drawCircle(Offset(24 * s, 11 * s), 5.5 * s, paint);
    final path = Path()
      ..moveTo(10 * s, 21 * s)
      ..lineTo(24 * s, 39 * s)
      ..lineTo(38 * s, 21 * s);
    canvas.drawPath(
      path,
      Paint()
        ..color = VColors.ink
        ..style = PaintingStyle.stroke
        ..strokeWidth = 7 * s
        ..strokeCap = StrokeCap.round
        ..strokeJoin = StrokeJoin.round,
    );
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

/// En-tête des étapes d'inscription : retour et points de progression.
class StepHeader extends StatelessWidget {
  const StepHeader({super.key, required this.step, this.total = 4, this.dark = false});

  final int step;
  final int total;
  final bool dark;

  @override
  Widget build(BuildContext context) {
    final color = dark ? Colors.white : VColors.ink;
    return Row(
      children: [
        IconButton(
          onPressed: () => Navigator.of(context).maybePop(),
          icon: Icon(Icons.arrow_back_ios_new_rounded, color: color, size: 22),
          tooltip: 'Retour',
        ),
        const Spacer(),
        Semantics(
          label: 'Étape $step sur $total',
          child: Row(
            children: [
              for (var i = 1; i <= total; i++)
                AnimatedContainer(
                  duration: const Duration(milliseconds: 250),
                  margin: const EdgeInsets.symmetric(horizontal: 3),
                  width: i == step ? 22 : 8,
                  height: 8,
                  decoration: BoxDecoration(
                    color: i <= step ? VColors.green : (dark ? VColors.dark3 : VColors.line),
                    borderRadius: BorderRadius.circular(4),
                  ),
                ),
            ],
          ),
        ),
        const Spacer(),
        const SizedBox(width: 48),
      ],
    );
  }
}

/// Pavé numérique sans bordures.
class Keypad extends StatelessWidget {
  const Keypad({super.key, required this.onDigit, required this.onDelete, this.onBiometric, this.keyHeight = 66});

  final ValueChanged<String> onDigit;
  final VoidCallback onDelete;
  final VoidCallback? onBiometric;
  final double keyHeight;

  @override
  Widget build(BuildContext context) {
    Widget key(Widget child, VoidCallback? onTap, {String? label}) => Expanded(
          child: SizedBox(
            height: keyHeight,
            child: Semantics(
              button: true,
              label: label,
              excludeSemantics: label != null,
              child: InkResponse(onTap: onTap, radius: 40, child: Center(child: child)),
            ),
          ),
        );
    Widget digit(String d) =>
        key(Text(d, style: const TextStyle(fontSize: 30, fontWeight: FontWeight.w600)), () => onDigit(d), label: d);

    return Column(
      children: [
        for (final row in const [
          ['1', '2', '3'],
          ['4', '5', '6'],
          ['7', '8', '9'],
        ])
          Row(children: [for (final d in row) digit(d)]),
        Row(children: [
          onBiometric == null
              ? const Expanded(child: SizedBox())
              : key(const Icon(Icons.fingerprint_rounded, size: 34, color: VColors.greenText), onBiometric,
                  label: 'Utiliser l\'empreinte'),
          digit('0'),
          key(const Icon(Icons.backspace_outlined, size: 26), onDelete, label: 'Effacer'),
        ]),
      ],
    );
  }
}

/// Points du code secret.
class PinDots extends StatelessWidget {
  const PinDots({super.key, required this.filled, this.length = 4, this.error = false});

  final int filled;
  final int length;
  final bool error;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: '$filled chiffres sur $length',
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          for (var i = 0; i < length; i++)
            AnimatedContainer(
              duration: const Duration(milliseconds: 150),
              margin: const EdgeInsets.symmetric(horizontal: 11),
              width: 22,
              height: 22,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: error ? VColors.danger : (i < filled ? VColors.ink : VColors.line),
              ),
            ),
        ],
      ),
    );
  }
}

/// Fond sombre avec des cercles discrets (accueil et bienvenue).
class DarkRings extends StatelessWidget {
  const DarkRings({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return ColoredBox(
      color: VColors.ink,
      child: Stack(
        children: [
          Positioned.fill(child: CustomPaint(painter: _RingsPainter())),
          child,
        ],
      ),
    );
  }
}

class _RingsPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final c = Offset(size.width / 2, size.height * 0.45);
    final stroke = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1;
    canvas.drawCircle(c, 320, stroke..color = Colors.white.withValues(alpha: 0.06));
    canvas.drawCircle(c, 240, stroke..color = Colors.white.withValues(alpha: 0.08));
    canvas.drawCircle(c, 160, Paint()..color = VColors.green.withValues(alpha: 0.10));
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

void showMessage(BuildContext context, String message) {
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(content: Text(message)));
}
