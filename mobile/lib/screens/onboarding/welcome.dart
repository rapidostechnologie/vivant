import 'package:flutter/material.dart';

import '../../theme.dart';
import '../../widgets/common.dart';
import 'phone.dart';

class WelcomeScreen extends StatelessWidget {
  const WelcomeScreen({super.key});

  @override
  Widget build(BuildContext context) {
    void start() => Navigator.of(context).push(MaterialPageRoute(builder: (_) => const PhoneScreen()));
    return Scaffold(
      body: DarkRings(
        child: SafeArea(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
            child: Column(
              children: [
                const Spacer(flex: 3),
                const VivantLogo(),
                const SizedBox(height: 36),
                const Text('Vivant',
                    style: TextStyle(color: Colors.white, fontSize: 44, fontWeight: FontWeight.w800, letterSpacing: -1.4)),
                const SizedBox(height: 10),
                const Text('Votre identité, prouvée en un geste.',
                    textAlign: TextAlign.center, style: TextStyle(color: VColors.mutedOnDark, fontSize: 18)),
                const Spacer(flex: 4),
                PillButton(label: 'Commencer', onPressed: start),
                const SizedBox(height: 6),
                TextButton(
                  onPressed: start,
                  style: TextButton.styleFrom(foregroundColor: Colors.white, minimumSize: const Size.fromHeight(52)),
                  child: const Text("J'ai déjà un compte", style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
