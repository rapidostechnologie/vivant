import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'app_state.dart';
import 'screens/home.dart';
import 'screens/lock.dart';
import 'screens/onboarding/welcome.dart';
import 'theme.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final state = AppState();
  await state.load();
  runApp(VivantApp(state: state));
}

class VivantApp extends StatefulWidget {
  const VivantApp({super.key, required this.state});

  final AppState state;

  @override
  State<VivantApp> createState() => _VivantAppState();
}

class _VivantAppState extends State<VivantApp> with WidgetsBindingObserver {
  final _navigator = GlobalKey<NavigatorState>();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  // L'application se verrouille dès qu'elle passe en arrière-plan.
  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.paused && widget.state.identity != null && widget.state.unlocked) {
      _navigator.currentState?.popUntil((r) => r.isFirst);
      widget.state.setUnlocked(false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return AppScope(
      state: widget.state,
      child: AnnotatedRegion<SystemUiOverlayStyle>(
        value: SystemUiOverlayStyle.dark,
        child: MaterialApp(
          title: 'Vivant',
          navigatorKey: _navigator,
          debugShowCheckedModeBanner: false,
          theme: buildTheme(),
          home: const _Gate(),
        ),
      ),
    );
  }
}

/// Premier écran : bienvenue, verrou ou accueil selon l'état.
class _Gate extends StatelessWidget {
  const _Gate();

  @override
  Widget build(BuildContext context) {
    final app = AppScope.of(context);
    final Widget screen;
    if (app.identity == null) {
      screen = const WelcomeScreen();
    } else if (!app.unlocked) {
      screen = const LockScreen();
    } else {
      screen = const HomeShell();
    }
    return AnimatedSwitcher(duration: const Duration(milliseconds: 250), child: screen);
  }
}
