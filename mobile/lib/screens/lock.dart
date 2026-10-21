import 'package:flutter/material.dart';

import '../app_state.dart';
import '../theme.dart';
import '../widgets/common.dart';
import '../widgets/pin_entry.dart';

/// Déverrouillage à l'ouverture de l'application.
class LockScreen extends StatelessWidget {
  const LockScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final app = AppScope.read(context);
    return Scaffold(
      body: SafeArea(
        child: Column(
          children: [
            const Spacer(),
            const VivantLogo(size: 64),
            const SizedBox(height: 24),
            Text(app.identity!.firstName.isEmpty ? 'Bonjour' : 'Bonjour ${app.identity!.firstName}',
                style: const TextStyle(fontSize: 28, fontWeight: FontWeight.w800, letterSpacing: -0.6)),
            const SizedBox(height: 6),
            const Text('Entrez votre code', style: VText.subtitle),
            const Spacer(),
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 0, 12, 24),
              child: PinEntry(onSuccess: () => app.setUnlocked(true), keyHeight: 66),
            ),
          ],
        ),
      ),
    );
  }
}
