import React from "react";
import styles from "./styles.module.css";
import ImageGallery from "react-image-gallery";
import "react-image-gallery/styles/css/image-gallery.css"

// https://github.com/xiaolin/react-image-gallery
// https://stackoverflow.com/questions/3746725/how-to-create-an-array-containing-1-n
const images = Array.from({length: 27}, (_, i) => {
        return {
            original: "/img/slides/atmos-intro-" + (i + 1) + ".svg",
            thumbnail: "/img/slides/atmos-intro-" + (i + 1) + ".svg"
        }
    }
);

export default function HomepageFeatures() {
    return (
        <section className={styles.features}>
            <div className="container">
                <ImageGallery items={images} thumbnailPosition={"left"} showBullets={false} showIndex={true} slideInterval={4000}/>
            </div>
        </section>
    );
}
