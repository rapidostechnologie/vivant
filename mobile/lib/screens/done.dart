import 'package:flutter/material.dart';

import '../theme.dart';
import '../widgets/common.dart';

class DoneScreen extends StatelessWidget {
  const DoneScreen({super.key, required this.clientName});

  final String clientName;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: VColors.green,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
          child: Column(
            children: [
              const Spacer(),
              TweenAnimationBuilder<double>(
                tween: Tween(begin: 0.4, end: 1),
                duration: const Duration(milliseconds: 450),
                curve: Curves.elasticOut,
                builder: (_, v, child) => Transform.scale(scale: v, child: child),
                child: const CircleAvatar(
                  radius: 56,
                  backgroundColor: VColors.ink,
                  child: Icon(Icons.check_rounded, color: VColors.green, size: 64),
                ),
              ),
              const SizedBox(height: 32),
              const Text("C'est fait !", style: TextStyle(fontSize: 40, fontWeight: FontWeight.w800, letterSpacing: -1.2)),
              const SizedBox(height: 10),
              Text('Retournez sur $clientName.',
                  textAlign: TextAlign.center, style: const TextStyle(fontSize: 18, color: VColors.ink)),
              const Spacer(),
              PillButton(label: 'Terminé', dark: true, onPressed: () => Navigator.of(context).pop()),
            ],
          ),
        ),
      ),
    );
  }
}
