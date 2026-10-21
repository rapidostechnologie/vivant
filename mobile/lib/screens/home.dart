import 'dart:async';

import 'package:flutter/material.dart';

import '../api/vivant_api.dart';
import '../app_state.dart';
import '../theme.dart';
import '../widgets/common.dart';
import 'activity.dart';
import 'confirm.dart';
import 'profile.dart';
import 'services.dart';

/// Coquille principale : Accueil, Services, Profil.
class HomeShell extends StatefulWidget {
  const HomeShell({super.key});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  var _tab = 0;

  @override
  Widget build(BuildContext context) {
    final dark = _tab == 0;
    return Scaffold(
      backgroundColor: dark ? VColors.ink : VColors.background,
      body: IndexedStack(
        index: _tab,
        children: [
          HomeTab(active: _tab == 0),
          const ServicesScreen(),
          const ProfileScreen(),
        ],
      ),
      bottomNavigationBar: _NavBar(index: _tab, dark: dark, onTap: (i) => setState(() => _tab = i)),
    );
  }
}

class _NavBar extends StatelessWidget {
  const _NavBar({required this.index, required this.dark, required this.onTap});

  final int index;
  final bool dark;
  final ValueChanged<int> onTap;

  @override
  Widget build(BuildContext context) {
    const items = [
      (Icons.home_rounded, 'Accueil'),
      (Icons.grid_view_rounded, 'Services'),
      (Icons.person_rounded, 'Profil'),
    ];
    return Container(
      decoration: BoxDecoration(
        color: dark ? VColors.ink : Colors.white,
        border: Border(top: BorderSide(color: dark ? VColors.dark2 : VColors.line)),
      ),
      child: SafeArea(
        top: false,
        child: SizedBox(
          height: 68,
          child: Row(
            children: [
              for (var i = 0; i < items.length; i++)
                Expanded(
                  child: Semantics(
                    selected: i == index,
                    button: true,
                    child: InkResponse(
                      onTap: () => onTap(i),
                      child: Column(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          Icon(items[i].$1,
                              size: 26,
                              color: i == index ? VColors.green : (dark ? VColors.mutedOnDark : VColors.muted)),
                          const SizedBox(height: 4),
                          Text(items[i].$2,
                              style: TextStyle(
                                fontSize: 12,
                                fontWeight: FontWeight.w700,
                                color: i == index ? (dark ? Colors.white : VColors.ink) : (dark ? VColors.mutedOnDark : VColors.muted),
                              )),
                        ],
                      ),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Accueil discret : un prénom, un statut, et les demandes à confirmer.
class HomeTab extends StatefulWidget {
  const HomeTab({super.key, required this.active});

  final bool active;

  @override
  State<HomeTab> createState() => _HomeTabState();
}

class _HomeTabState extends State<HomeTab> with WidgetsBindingObserver {
  Timer? _timer;
  PendingRequest? _request;
  var _opening = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _timer = Timer.periodic(const Duration(seconds: 3), (_) => _poll());
    _poll();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _timer?.cancel();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) _poll();
  }

  Future<void> _poll() async {
    if (!widget.active || _opening) return;
    final app = AppScope.read(context);
    final id = app.identity;
    if (id == null) return;
    try {
      final list = await app.api.pendingRequests(id);
      if (!mounted) return;
      setState(() => _request = list.isEmpty ? null : list.first);
    } on ApiException catch (e) {
      // Téléphone désactivé ailleurs (nouveau téléphone, révocation) : on oublie.
      if (e.status == 401 && e.code == 'unauthorized_device') await app.forget();
    }
  }

  Future<void> _open(PendingRequest r) async {
    _opening = true;
    await Navigator.of(context).push(MaterialPageRoute(builder: (_) => ConfirmScreen(request: r)));
    _opening = false;
    if (mounted) setState(() => _request = null);
    _poll();
  }

  @override
  Widget build(BuildContext context) {
    final app = AppScope.of(context);
    final name = app.identity?.firstName ?? '';
    final request = _request;
    return DarkRings(
      child: SafeArea(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          child: Column(
            children: [
              const SizedBox(height: 8),
              Row(
                children: [
                  const VivantLogo(size: 36),
                  const Spacer(),
                  IconButton(
                    tooltip: 'Activité',
                    onPressed: () => Navigator.of(context).push(MaterialPageRoute(builder: (_) => const ActivityScreen())),
                    icon: const Icon(Icons.notifications_none_rounded, color: Colors.white, size: 28),
                  ),
                ],
              ),
              const Spacer(flex: 2),
              Text(
                name.isEmpty ? 'Bonjour' : 'Bonjour,\n$name',
                textAlign: TextAlign.center,
                style: const TextStyle(
                  fontFamily: 'serif',
                  color: Colors.white,
                  fontSize: 46,
                  height: 1.05,
                  fontWeight: FontWeight.w500,
                ),
              ),
              const SizedBox(height: 20),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
                decoration: BoxDecoration(color: VColors.dark2, borderRadius: BorderRadius.circular(40)),
                child: const Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(Icons.verified_rounded, color: VColors.green, size: 18),
                    SizedBox(width: 8),
                    Text('Identité vérifiée', style: TextStyle(color: Colors.white, fontWeight: FontWeight.w700)),
                  ],
                ),
              ),
              const Spacer(flex: 3),
              AnimatedSwitcher(
                duration: const Duration(milliseconds: 300),
                transitionBuilder: (child, a) => SlideTransition(
                  position: Tween(begin: const Offset(0, 0.4), end: Offset.zero).animate(a),
                  child: FadeTransition(opacity: a, child: child),
                ),
                child: request == null
                    ? const Padding(
                        key: ValueKey('idle'),
                        padding: EdgeInsets.only(bottom: 28),
                        child: Text('Aucune demande en attente',
                            style: TextStyle(color: VColors.mutedOnDark, fontSize: 15)),
                      )
                    : _RequestCard(key: ValueKey(request.id), request: request, onOpen: () => _open(request)),
              ),
              const SizedBox(height: 16),
            ],
          ),
        ),
      ),
    );
  }
}

class _RequestCard extends StatelessWidget {
  const _RequestCard({super.key, required this.request, required this.onOpen});

  final PendingRequest request;
  final VoidCallback onOpen;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(color: Colors.white, borderRadius: BorderRadius.circular(28)),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 48,
                height: 48,
                alignment: Alignment.center,
                decoration: BoxDecoration(color: VColors.greenSoft, borderRadius: BorderRadius.circular(14)),
                child: Text(request.clientName.characters.first.toUpperCase(),
                    style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w800, color: VColors.greenText)),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text("S'identifier", style: TextStyle(color: VColors.muted, fontSize: 14)),
                    Text(request.clientName,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(fontSize: 19, fontWeight: FontWeight.w800)),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 18),
          PillButton(label: 'Confirmer', onPressed: onOpen),
        ],
      ),
    );
  }
}
