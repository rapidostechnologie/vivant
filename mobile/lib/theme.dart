import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';

/// Couleurs de Vivant (voir docs/MASTER.md, section Design).
abstract final class VColors {
  static const green = Color(0xFF34D186);
  static const greenHover = Color(0xFF2BBE77);
  static const greenText = Color(0xFF12814B);
  static const greenSoft = Color(0xFFEAFAF2);
  static const greenLight = Color(0xFF7FE3B2);
  static const ink = Color(0xFF0F1113);
  static const muted = Color(0xFF5F6368);
  static const line = Color(0xFFE6E8EB);
  static const soft = Color(0xFFF2F3F5);
  static const background = Color(0xFFF6F7F8);
  static const dark2 = Color(0xFF1C1F22);
  static const dark3 = Color(0xFF2E3236);
  static const mutedOnDark = Color(0xFFA2A7AD);
  static const danger = Color(0xFFDC2626);
}

ThemeData buildTheme() {
  final base = ThemeData(
    useMaterial3: true,
    colorScheme: ColorScheme.fromSeed(
      seedColor: VColors.green,
      primary: VColors.green,
      onPrimary: VColors.ink,
      surface: Colors.white,
      onSurface: VColors.ink,
      error: VColors.danger,
    ),
    scaffoldBackgroundColor: Colors.white,
  );
  return base.copyWith(
    textTheme: base.textTheme.apply(bodyColor: VColors.ink, displayColor: VColors.ink),
    snackBarTheme: const SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      backgroundColor: VColors.ink,
      contentTextStyle: TextStyle(color: Colors.white, fontSize: 15, fontWeight: FontWeight.w600),
    ),
    pageTransitionsTheme: const PageTransitionsTheme(builders: {
      TargetPlatform.android: CupertinoPageTransitionsBuilder(),
      TargetPlatform.iOS: CupertinoPageTransitionsBuilder(),
    }),
  );
}

/// Styles de texte communs.
abstract final class VText {
  static const title = TextStyle(fontSize: 36, height: 1.05, fontWeight: FontWeight.w800, letterSpacing: -1.1);
  static const subtitle = TextStyle(fontSize: 17, color: VColors.muted);
  static const section = TextStyle(fontSize: 13, fontWeight: FontWeight.w700, letterSpacing: 0.8, color: VColors.muted);
}
