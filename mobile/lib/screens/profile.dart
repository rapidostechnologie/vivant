import 'package:flutter/material.dart';

import '../app_state.dart';
import '../theme.dart';
import '../widgets/common.dart';
import '../widgets/page.dart';
import '../widgets/pin_entry.dart';
import 'activity.dart';

/// Profil : rien d'exposé par défaut, les données s'ouvrent avec le code.
class ProfileScreen extends StatelessWidget {
  const ProfileScreen({super.key});

  void _push(BuildContext context, Widget page) =>
      Navigator.of(context).push(MaterialPageRoute(builder: (_) => page));

  @override
  Widget build(BuildContext context) {
    final app = AppScope.of(context);
    final name = app.identity?.firstName ?? '';
    return SubPage(
      title: 'Profil',
      back: false,
      children: [
        Row(
          children: [
            CircleAvatar(
              radius: 30,
              backgroundColor: VColors.green,
              child: Text(name.isEmpty ? 'V' : name.characters.first,
                  style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w800, color: VColors.ink)),
            ),
            const SizedBox(width: 16),
            Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(name, style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w800)),
                const SizedBox(height: 2),
                const Row(children: [
                  Icon(Icons.verified_rounded, size: 16, color: VColors.greenText),
                  SizedBox(width: 6),
                  Text('Identité vérifiée', style: TextStyle(color: VColors.greenText, fontWeight: FontWeight.w700)),
                ]),
              ],
            ),
          ],
        ),
        const SizedBox(height: 28),
        RowGroup(children: [
          RowItem(
            icon: Icons.badge_outlined,
            label: 'Mes données',
            onTap: () async {
              if (await askPin(context) && context.mounted) _push(context, const MyDataScreen());
            },
          ),
          RowItem(icon: Icons.history_rounded, label: 'Activité', onTap: () => _push(context, const ActivityScreen())),
        ]),
        RowGroup(children: [
          RowItem(icon: Icons.lock_outline_rounded, label: 'Sécurité', onTap: () => _push(context, const SecurityScreen())),
          RowItem(icon: Icons.shield_outlined, label: 'Confidentialité', onTap: () => _push(context, const PrivacyScreen())),
          RowItem(icon: Icons.help_outline_rounded, label: 'Aide', onTap: () => _push(context, const HelpScreen())),
        ]),
        RowGroup(children: [
          RowItem(
            icon: Icons.logout_rounded,
            label: 'Retirer Vivant de ce téléphone',
            danger: true,
            onTap: () => _push(context, const DeleteScreen()),
          ),
        ]),
        const SizedBox(height: 8),
        const Center(child: Text('Vivant 0.1.0', style: TextStyle(color: VColors.muted, fontSize: 13))),
      ],
    );
  }
}

/// Données d'identité, visibles seulement après le code secret.
class MyDataScreen extends StatefulWidget {
  const MyDataScreen({super.key});

  @override
  State<MyDataScreen> createState() => _MyDataScreenState();
}

class _MyDataScreenState extends State<MyDataScreen> {
  late final _me = AppScope.read(context).api.me(AppScope.read(context).identity!);

  @override
  Widget build(BuildContext context) {
    return FutureBuilder(
      future: _me,
      builder: (context, snap) {
        final me = snap.data;
        final id = (me?['identity'] as Map<String, dynamic>?) ?? const {};
        String v(String k) => (id[k] as String?)?.trim().isNotEmpty == true ? id[k] as String : '—';
        return SubPage(
          title: 'Mes données',
          children: [
            if (me == null)
              snap.hasError
                  ? Text(snap.error.toString(), style: VText.subtitle)
                  : const Center(child: Padding(padding: EdgeInsets.all(40), child: CircularProgressIndicator()))
            else ...[
              RowGroup(children: [
                RowItem(icon: Icons.person_outline_rounded, label: 'Nom', value: '${v('given_names')} ${v('family_name')}'),
                RowItem(icon: Icons.cake_outlined, label: 'Naissance', value: '${v('birth_date')} · ${v('birth_place')}'),
                RowItem(icon: Icons.fingerprint_rounded, label: 'NIN', value: v('nin')),
                RowItem(icon: Icons.phone_outlined, label: 'Téléphone', value: me['phone'] as String? ?? '—'),
              ]),
              const Text('APPAREILS', style: VText.section),
              const SizedBox(height: 10),
              RowGroup(children: [
                for (final d in (me['devices'] as List? ?? const []))
                  if (d['revoked_at'] == null)
                    RowItem(
                      icon: Icons.smartphone_rounded,
                      label: d['name'] as String,
                      value: d['current'] == true ? 'Ce téléphone' : null,
                    ),
              ]),
            ],
          ],
        );
      },
    );
  }
}

class SecurityScreen extends StatelessWidget {
  const SecurityScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return SubPage(
      title: 'Sécurité',
      children: [
        RowGroup(children: [
          RowItem(
            icon: Icons.pin_outlined,
            label: 'Changer le code secret',
            onTap: () async {
              if (!await askPin(context, title: 'Code actuel') || !context.mounted) return;
              final pin = await _newPin(context);
              if (pin == null || !context.mounted) return;
              await AppScope.read(context).store.setPin(pin);
              if (context.mounted) showMessage(context, 'Code secret changé.');
            },
          ),
        ]),
        const Padding(
          padding: EdgeInsets.symmetric(horizontal: 4),
          child: Text(
            'Votre clé reste dans ce téléphone. Personne, pas même Vivant, ne peut confirmer à votre place.',
            style: TextStyle(color: VColors.muted, height: 1.4),
          ),
        ),
      ],
    );
  }

  static Future<String?> _newPin(BuildContext context) {
    var pin = '';
    return showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      backgroundColor: Colors.white,
      builder: (context) => StatefulBuilder(
        builder: (context, setState) => SafeArea(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(12, 0, 12, 16),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Text('Nouveau code', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800)),
                const SizedBox(height: 24),
                PinDots(filled: pin.length),
                const SizedBox(height: 36),
                Keypad(
                  keyHeight: 60,
                  onDigit: (d) {
                    setState(() => pin += d);
                    if (pin.length == 4) Navigator.pop(context, pin);
                  },
                  onDelete: () => setState(() => pin = pin.isEmpty ? '' : pin.substring(0, pin.length - 1)),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class PrivacyScreen extends StatelessWidget {
  const PrivacyScreen({super.key});

  @override
  Widget build(BuildContext context) {
    const points = [
      (Icons.visibility_off_outlined, 'Rien sans votre accord', 'Chaque partage se confirme sur ce téléphone.'),
      (Icons.filter_alt_outlined, 'Le strict nécessaire', 'Un service ne reçoit que ce qu\'il demande.'),
      (Icons.link_off_rounded, 'Pas de pistage', 'Chaque service vous voit sous un identifiant différent.'),
    ];
    return SubPage(
      title: 'Confidentialité',
      children: [
        RowGroup(children: [
          for (final p in points)
            ListTile(
              minTileHeight: 72,
              leading: Icon(p.$1, color: VColors.greenText),
              title: Text(p.$2, style: const TextStyle(fontWeight: FontWeight.w700)),
              subtitle: Text(p.$3, style: const TextStyle(color: VColors.muted)),
            ),
        ]),
      ],
    );
  }
}

class HelpScreen extends StatelessWidget {
  const HelpScreen({super.key});

  @override
  Widget build(BuildContext context) {
    const faq = [
      ('Comment me connecter à un site ?', 'Choisissez « Vivant » sur le site, puis confirmez ici avec le code affiché.'),
      ('Je change de téléphone', 'Installez Vivant sur le nouveau et refaites l\'inscription. L\'ancien est désactivé.'),
      ('On m\'a volé mon téléphone', 'Sans votre code, personne ne peut confirmer. Réinscrivez-vous sur un autre téléphone pour le désactiver.'),
      ('Je n\'ai rien demandé', 'Refusez. Ne confirmez jamais une demande que vous n\'avez pas faite.'),
    ];
    return SubPage(
      title: 'Aide',
      children: [
        RowGroup(children: [
          for (final q in faq)
            ExpansionTile(
              shape: const Border(),
              title: Text(q.$1, style: const TextStyle(fontWeight: FontWeight.w700)),
              childrenPadding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
              expandedAlignment: Alignment.centerLeft,
              children: [Text(q.$2, style: const TextStyle(color: VColors.muted, height: 1.4))],
            ),
        ]),
      ],
    );
  }
}

class DeleteScreen extends StatefulWidget {
  const DeleteScreen({super.key});

  @override
  State<DeleteScreen> createState() => _DeleteScreenState();
}

class _DeleteScreenState extends State<DeleteScreen> {
  var _busy = false;

  Future<void> _delete() async {
    final app = AppScope.read(context);
    final nav = Navigator.of(context);
    if (!await askPin(context, title: 'Confirmez avec votre code')) return;
    setState(() => _busy = true);
    try {
      await app.forget();
      nav.popUntil((r) => r.isFirst);
    } catch (e) {
      if (mounted) showMessage(context, e.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(backgroundColor: Colors.white, surfaceTintColor: Colors.transparent),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text('Retirer Vivant\nde ce téléphone ?', style: VText.title),
              const SizedBox(height: 12),
              const Text('Ce téléphone ne pourra plus confirmer votre identité. Vous pourrez vous réinscrire.',
                  style: VText.subtitle),
              const Spacer(),
              PillButton(label: 'Retirer', danger: true, loading: _busy, onPressed: _delete),
              const SizedBox(height: 8),
              SizedBox(
                width: double.infinity,
                child: TextButton(
                  onPressed: () => Navigator.of(context).pop(),
                  style: TextButton.styleFrom(minimumSize: const Size.fromHeight(56), foregroundColor: VColors.ink),
                  child: const Text('Annuler', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
