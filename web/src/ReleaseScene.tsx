import { useEffect, useRef } from 'react';
import {
  AmbientLight,
  Color,
  DirectionalLight,
  Group,
  Mesh,
  MeshStandardMaterial,
  PerspectiveCamera,
  Scene,
  TorusGeometry,
  WebGLRenderer,
  EdgesGeometry,
  LineBasicMaterial,
  LineSegments,
  IcosahedronGeometry,
  PointLight,
} from 'three';

export default function ReleaseScene({ blocked }: { blocked: boolean }) {
  const host = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const element = host.current;
    if (!element) return;
    let renderer: WebGLRenderer;
    try {
      renderer = new WebGLRenderer({ alpha: true, antialias: true });
    } catch {
      return;
    }

    renderer.setPixelRatio(Math.min(devicePixelRatio || 1, 2));
    element.appendChild(renderer.domElement);
    const scene = new Scene();
    const camera = new PerspectiveCamera(34, 1, 0.1, 40);
    camera.position.set(0, 0, 8.4);
    const sculpture = new Group();
    scene.add(sculpture);
    const ringMaterial = new MeshStandardMaterial({
      color: '#849bff',
      metalness: 0.65,
      roughness: 0.24,
      emissive: '#18235f',
    });
    const coreMaterial = new MeshStandardMaterial({
      color: blocked ? '#e78c99' : '#86b2ff',
      metalness: 0.5,
      roughness: 0.23,
      emissive: blocked ? '#42192f' : '#203775',
      flatShading: true,
    });
    const ring = new TorusGeometry(1.32, 0.035, 10, 100);
    const outer = new TorusGeometry(1.73, 0.016, 8, 100);
    const firstRing = new Mesh(ring, ringMaterial);
    const secondRing = new Mesh(ring, ringMaterial);
    const thirdRing = new Mesh(outer, ringMaterial);
    firstRing.rotation.x = 0.95;
    secondRing.rotation.set(0.3, 1.1, -0.3);
    thirdRing.rotation.set(1.1, 0.6, 0.2);
    const coreGeometry = new IcosahedronGeometry(0.81, 0);
    const core = new Mesh(coreGeometry, coreMaterial);
    const edges = new EdgesGeometry(coreGeometry);
    const edgeMaterial = new LineBasicMaterial({
      color: new Color('#d0dcff'),
      transparent: true,
      opacity: 0.65,
    });
    core.add(new LineSegments(edges, edgeMaterial));
    sculpture.add(firstRing, secondRing, thirdRing, core);
    scene.add(new AmbientLight('#bbcaff', 2));
    const key = new DirectionalLight('#e2ebff', 4);
    key.position.set(2, 3, 4);
    scene.add(key);
    const rim = new PointLight('#8f64ff', 23);
    rim.position.set(-3, -2, 2);
    scene.add(rim);
    const motion = matchMedia('(prefers-reduced-motion: reduce)');
    const resize = () => {
      const { clientWidth: width, clientHeight: height } = element;
      if (!width || !height) return;
      camera.aspect = width / height;
      camera.updateProjectionMatrix();
      renderer.setSize(width, height, false);
      renderer.render(scene, camera);
    };
    const observer = new ResizeObserver(resize);
    observer.observe(element);
    sculpture.rotation.set(0.25, -0.45, 0.15);
    resize();
    renderer.setAnimationLoop((time) => {
      if (motion.matches) return;
      sculpture.rotation.y = -0.45 + Math.sin(time / 3300) * 0.38;
      sculpture.rotation.x = 0.25 + Math.sin(time / 4700) * 0.22;
      core.rotation.y = time / 5500;
      renderer.render(scene, camera);
    });
    return () => {
      observer.disconnect();
      renderer.setAnimationLoop(null);
      for (const geometry of [ring, outer, coreGeometry, edges])
        geometry.dispose();
      for (const material of [ringMaterial, coreMaterial, edgeMaterial])
        material.dispose();
      renderer.dispose();
      renderer.domElement.remove();
    };
  }, [blocked]);
  return <div className="scene" aria-hidden="true" ref={host} />;
}
